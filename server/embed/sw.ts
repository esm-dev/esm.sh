/*! esm.sh/sw - speeding up your modern(es2015+) web application with service worker.
 *  Docs: https://docs.esm.sh/sw
 */

/// <reference lib="webworker" />

import type { ArchiveEntry, FireOptions, Plugin, SW } from "./types/sw.d.ts";

const doc: Document | undefined = globalThis.document;
const kSw = "esm.sh/sw";
const kTypeEsmArchive = "application/esm-archive";

/**
 * class `Archive` implements the reader for esm-archive format.
 * more details see https://www.npmjs.com/package/esm-archive
 */
class Archive {
  private _buf: ArrayBuffer;
  private _files: Record<string, ArchiveEntry> = {};

  static invalidFormat = new Error("Invalid esm-archive format");

  constructor(buffer: ArrayBuffer) {
    this._buf = buffer;
    this._parse();
  }

  public checksum: number;

  private _parse() {
    const dv = new DataView(this._buf);
    const decoder = new TextDecoder();
    const readUint32 = (offset: number) => dv.getUint32(offset);
    const readString = (offset: number, length: number) => decoder.decode(new Uint8Array(this._buf, offset, length));
    if (this._buf.byteLength < 18 || readString(0, 10) !== "ESMARCHIVE") {
      throw Archive.invalidFormat;
    }
    const length = readUint32(10);
    if (length !== this._buf.byteLength) {
      throw Archive.invalidFormat;
    }
    this.checksum = readUint32(14);
    let offset = 18;
    while (offset < dv.byteLength) {
      const nameLen = dv.getUint16(offset);
      offset += 2;
      const name = readString(offset, nameLen);
      offset += nameLen;
      const typeLen = dv.getUint8(offset);
      offset += 1;
      const type = readString(offset, typeLen);
      offset += typeLen;
      const lastModified = readUint32(offset) * 1000; // convert to ms
      offset += 4;
      const size = readUint32(offset);
      offset += 4;
      this._files[name] = { name, type, lastModified, offset, size };
      offset += size;
    }
  }

  exists(filename: string) {
    return filename in this._files;
  }

  openFile(filename: string) {
    const { name, offset, size, ...rest } = this._files[filename];
    return new File([this._buf.slice(offset, offset + size)], name, rest);
  }
}

/** class `SWImpl` implements the `SW` interface. */
class SWImpl implements SW {
  private _swActive: ServiceWorker | null = null;
  private _archive: Archive | null = null;
  private _fetchListeners: ((event: FetchEvent) => void)[] = [];
  private _fireListeners: ((sw: ServiceWorker) => void)[] = [];
  private _promises: Promise<any>[] = [];

  use(...plugins: readonly Plugin[]) {
    plugins.forEach((plugin) => plugin(this));
    return this;
  }

  onUpdateFound = () => location.reload();

  onFetch(handler: (event: FetchEvent) => void) {
    this._fetchListeners.push(handler);
    return this;
  }

  onFire(handler: (reg: ServiceWorker) => void) {
    if (this._swActive) {
      handler(this._swActive);
    } else {
      this._fireListeners.push(handler);
    }
    return this;
  }

  waitUntil(...promises: readonly Promise<void>[]) {
    this._promises.push(...promises);
    return this;
  }

  async fire(options: FireOptions = {}) {
    const serviceWorker = navigator.serviceWorker;
    if (!serviceWorker) {
      panic("Service Worker not supported");
    }

    // add preload link for the main module if it's provided
    options.main && appendElement("link", { rel: "modulepreload", href: options.main });

    // detect controller change
    serviceWorker.oncontrollerchange = () => this.onUpdateFound();

    return new Promise<ServiceWorker>(async (resolve, reject) => {
      const swr = await serviceWorker.register(options.sw?.module ?? "/sw.js", {
        type: "module",
        updateViaCache: options.sw?.updateViaCache,
      });
      const onActive = (firstInstall = false) => {
        if (swr.active?.state === "activated") {
          const active = swr.active;
          this._onFire(active, firstInstall).then(() => {
            // add main script tag if it's provided
            options.main && appendElement("script", { type: "module", src: options.main });
            resolve(active);
          }).catch(reject);
        }
      };

      // detect Service Worker install/update available and wait for it to become installed
      swr.onupdatefound = () => {
        const installing = swr.installing;
        if (installing) {
          installing.onerror = reject;
          installing.onstatechange = () => {
            const waiting = swr.waiting;
            // it's first install
            if (waiting && !serviceWorker.controller) {
              waiting.onstatechange = () => onActive(true);
            }
          };
        }
      };

      // fire app immediately if there's an activated Service Worker
      onActive();
    });
  }

  private async _onFire(swActive: ServiceWorker, firstInstall = false) {
    // download esm-archive and and send it to the Service Worker if has any
    queryElements<HTMLLinkElement>(`link[type^="${kTypeEsmArchive}"][href]`, (el, i) => {
      const p = fetch(el.href).then((res) => {
        if (!res.ok) {
          panic("Failed to download esm-archive: " + (res.statusText ?? res.status));
        }
        return res.arrayBuffer();
      }).then(async (arrayBuffer) => {
        const checksum = el.getAttribute("checksum");
        if (checksum) {
          const buf = await crypto.subtle.digest("SHA-256", arrayBuffer);
          if (btoa(String.fromCharCode(...new Uint8Array(buf))) !== checksum) {
            panic("Checksum mismatch: " + checksum);
          }
        }
        return new Promise<void>((resolve, reject) => {
          new BroadcastChannel(kSw).onmessage = ({ data }) => {
            if (data === 0) {
              reject(new Error("Invalid esm-archive format"));
            } else if (data === 1) {
              if (firstInstall) {
                resolve();
              } else {
                this.onUpdateFound();
              }
            } else if (data === 2) {
              resolve();
            }
          };
          swActive.postMessage([i, arrayBuffer, el.type.endsWith("+gzip")]);
        });
      });
      // if it's first install, wait until the download finished
      if (firstInstall) {
        this._promises.push(p);
      }
    });

    // wait until all promises resolved
    if (this._promises.length > 0) {
      await Promise.all(this._promises);
    }

    // fire all `fire` listeners
    for (const handler of this._fireListeners) {
      handler(swActive);
    }

    // apply script tags with type="esm" to type="module"
    queryElements<HTMLScriptElement>("script[type='esm']", (el) => {
      const copy = el.cloneNode(true) as HTMLScriptElement;
      copy.type = "module";
      el.replaceWith(copy);
    });

    this._promises = [];
    this._fireListeners = [];
    this._swActive = swActive;
  }

  listen() {
    // @ts-expect-error missing types
    if (typeof clients === "undefined") {
      panic("Service Worker scope not found.");
    }

    const on: typeof addEventListener = addEventListener;
    const bc = new BroadcastChannel(kSw);
    const cache = caches.open(kSw);
    this._promises.push(
      cache.then((cache) =>
        cache.match("/" + kTypeEsmArchive).then((res) =>
          res?.arrayBuffer().then((buf) => {
            try {
              this._archive = new Archive(buf);
            } catch (err) {
              // ignore
            }
          })
        )
      ),
    );

    on("install", (evt) => {
      // @ts-expect-error missing types
      skipWaiting();
      evt.waitUntil(Promise.all(this._promises));
    });

    on("activate", (evt) => {
      // @ts-expect-error missing types
      evt.waitUntil(clients.claim());
    });

    on("fetch", (evt) => {
      const { request } = evt as FetchEvent;
      const url = new URL(request.url);
      const { pathname } = url;
      const isSameOrigin = url.origin === location.origin;
      const pathOrHref = isSameOrigin ? pathname : request.url;
      const archive = this._archive;
      const listeners = this._fetchListeners;
      const respondWith = (res: Response | Promise<Response>) => evt.respondWith(res);
      if (archive?.exists(pathOrHref)) {
        const file = archive.openFile(pathOrHref)!;
        respondWith(createResponse(file, { "content-type": file.type }));
      } else if (listeners.length > 0) {
        let responded = false;
        evt.respondWith = (res: Response | Promise<Response>) => {
          responded = true;
          respondWith(res);
        };
        for (const handler of listeners) {
          if (responded) {
            break;
          }
          handler(evt);
        }
      }
    });

    on("message", async (evt) => {
      const { data } = evt;
      if (Array.isArray(data)) {
        const [_idx, buffer, gz] = data;
        if (buffer instanceof ArrayBuffer) {
          try {
            let data = buffer;
            if (gz) {
              data = await createResponse(
                createResponse(buffer).body!.pipeThrough(new DecompressionStream("gzip")),
              ).arrayBuffer();
            }
            const archive = new Archive(data);
            const currentArchive = this._archive;
            const stale = !currentArchive || archive.checksum !== currentArchive.checksum;
            if (stale) {
              this._archive = archive;
              cache.then((cache) => cache.put("/" + kTypeEsmArchive, createResponse(data)));
            }
            bc.postMessage(stale ? 1 : 2);
          } catch (err) {
            bc.postMessage(0);
            console.error(err);
          }
        }
      }
    });
  }
}

/** query all elements by the given selectors. */
function queryElements<T extends Element>(
  selectors: string,
  callback: (value: T, index: number) => void,
) {
  // @ts-expect-error throw error if the `document` is undefined
  doc.querySelectorAll(selectors).forEach(callback);
}

/** create a response object. */
function createResponse(
  body: BodyInit | null,
  headers: HeadersInit = {},
  status = 200,
): Response {
  return new Response(body, { headers, status });
}

/** append an element to the head. */
function appendElement(tagName: string, attrs: Record<string, string>) {
  const el = doc!.createElement(tagName);
  for (const [k, v] of Object.entries(attrs)) {
    el[k] = v;
  }
  doc!.head.appendChild(el);
}

/** panic with the given message. */
function panic(message: string) {
  throw new Error(message);
}

export const sw = new SWImpl();
export default sw;

/*! 🔥 esm.sh/hot - speeding up your modern(es2015+) web application.
 *  Docs: https://docs.esm.sh/hot
 */

/// <reference lib="webworker" />

import type { ArchiveEntry, FireOptions, HotAPI, Plugin } from "./types/hot";

const doc: Document | undefined = globalThis.document;
const kHot = "esm.sh/hot";
const kMessage = "message";
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

/** class `Hot` implements the `HotAPI` interface. */
class Hot implements HotAPI {
  private _swModule: string | null = null;
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
    const sw = navigator.serviceWorker;
    if (!sw) {
      throw new Error("Service Worker not supported");
    }

    const { main, swModule = "/sw.js", swUpdateViaCache } = options;
    if (this._swModule === swModule) {
      throw new Error("Service Worker already registered");
    }
    this._swModule = swModule;

    // add preload link for the main module if it's provided
    if (main) {
      appendElement("link", { rel: "modulepreload", href: main });
    }

    // register Service Worker
    const swr = await sw.register(this._swModule, {
      type: "module",
      updateViaCache: swUpdateViaCache,
    });

    let resolve: (sw: ServiceWorker) => void;
    let reject: (err: any) => void;
    let promise = new Promise<ServiceWorker>((res, rej) => {
      resolve = res;
      reject = rej;
    });
    const tryFireApp = (firstInstall = false) => {
      if (swr.active?.state === "activated") {
        const { active } = swr;
        this._fireApp(active, firstInstall).then(() => {
          main && appendElement("script", { type: "module", src: main });
          resolve(active);
        }).catch(reject);
      }
    };

    // detect Service Worker update available and wait for it to become installed
    swr.onupdatefound = () => {
      const { installing } = swr;
      if (installing) {
        installing.onerror = reject;
        installing.onstatechange = () => {
          const { waiting } = swr;
          // it's first install
          if (waiting && !sw.controller) {
            waiting.onstatechange = () => tryFireApp(true);
          }
        };
      }
    };

    // detect controller change
    sw.oncontrollerchange = this.onUpdateFound;

    // fire app immediately if there's an activated Service Worker
    tryFireApp();

    return promise;
  }

  private async _fireApp(swActive: ServiceWorker, firstInstall = false) {
    // download esm archive and and send it to the Service Worker if has any
    queryElements<HTMLLinkElement>(`link[type^="${kTypeEsmArchive}"][href]`, (el, i) => {
      const p = fetch(el.href).then((res) => {
        if (res.ok) {
          if (el.type.endsWith("+gzip")) {
            res = new Response(res.body?.pipeThrough(new DecompressionStream("gzip")));
          }
          return res.arrayBuffer();
        }
        return Promise.reject(new Error(res.statusText ?? `<${res.status}>`));
      }).then(async (arrayBuffer) => {
        const checksum = el.getAttribute("checksum");
        if (checksum) {
          const buf = await crypto.subtle.digest("SHA-256", arrayBuffer);
          if (btoa(String.fromCharCode(...new Uint8Array(buf))) !== checksum) {
            throw new Error("Checksum mismatch: " + checksum);
          }
        }
        new BroadcastChannel(kHot).onmessage = (evt) => evt.data === 1 && this.onUpdateFound();
        swActive.postMessage([i, arrayBuffer]);
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

    // apply "[type=hot/module]" script tags
    queryElements<HTMLScriptElement>("script[type='hot/module']", (el) => {
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
      throw new Error("Service Worker scope not found.");
    }

    const on: typeof addEventListener = addEventListener;
    const cache = caches.open(kHot);
    this._promises.push(
      cache.then((cache) =>
        cache.match("/" + kTypeEsmArchive).then((res) => {
          res?.arrayBuffer().then((buf) => {
            this._archive = new Archive(buf);
          });
        })
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
      const respondWith = evt.respondWith.bind(evt);
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

    on(kMessage, (evt) => {
      const { data } = evt;
      if (Array.isArray(data)) {
        const [_idx, buffer] = data;
        if (buffer instanceof ArrayBuffer) {
          try {
            const archive = new Archive(buffer);
            if (archive.checksum !== this._archive?.checksum) {
              this._archive = archive;
              new BroadcastChannel(kHot).postMessage(1);
              cache.then((cache) => cache.put("/" + kTypeEsmArchive, createResponse(buffer)));
            }
          } catch (err) {
            console.error(err[kMessage]);
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
  // @ts-expect-error throw error if document is not available
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

/** append an element to the document. */
function appendElement(tag: string, attrs: Record<string, string>, parent: "head" | "body" = "head") {
  const el = doc!.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    el[k] = v;
  }
  doc![parent].appendChild(el);
}

export const hot = new Hot();
export default hot;

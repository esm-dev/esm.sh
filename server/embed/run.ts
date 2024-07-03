/*! 🔥 esm.sh/run - speeding up your modern(es2015+) web application with service worker.
 *  Docs: https://docs.esm.sh/run
 */

import type { ArchiveEntry, FireOptions, InstallOptions } from "./types/run.d.ts";

const doc: Document | undefined = globalThis.document;
const kSw = "esm.sh/run";
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

export async function install({
  main,
  onUpdateFound = () => location.reload(),
  swModule,
  swScope,
  swUpdateViaCache,
}: InstallOptions = {}): Promise<ServiceWorker> {
  const serviceWorker = navigator.serviceWorker;
  if (!serviceWorker) {
    panic("Service Worker not supported");
  }

  // add preload link for the main module if it's provided
  main && appendElement("link", { rel: "modulepreload", href: main });

  // detect controller change
  serviceWorker.oncontrollerchange = onUpdateFound;

  return new Promise<ServiceWorker>(async (resolve, reject) => {
    const swr = await serviceWorker.register(swModule ?? "/sw.js", {
      type: "module",
      scope: swScope,
      updateViaCache: swUpdateViaCache,
    });
    const run = async (firstInstall = false) => {
      if (swr.active?.state === "activated") {
        let dl: Promise<void> | undefined;
        // download esm-archive and and send it to the Service Worker if has any
        queryElement<HTMLLinkElement>(`link[type^="${kTypeEsmArchive}"][href]`, (el) => {
          dl = fetch(el.href).then((res) => {
            if (!res.ok) {
              panic("Failed to download esm-archive: " + (res.statusText ?? res.status));
            }
            return res.arrayBuffer();
          }).then(async (arrayBuffer) => {
            const checksum = getAttr(el, "checksum");
            if (checksum) {
              const buf = await crypto.subtle.digest("SHA-256", arrayBuffer);
              if (btoa(String.fromCharCode(...new Uint8Array(buf))) !== checksum) {
                panic("Invalid esm-archive: the checksum does not match");
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
                    onUpdateFound();
                  }
                } else if (data === 2) {
                  resolve();
                }
              };
              swr.active!.postMessage([0x127, arrayBuffer, el.type.endsWith("+gzip")]);
            });
          });
        });
        // if it's first install, wait until the esm-archive downloaded
        if (firstInstall && dl) {
          await dl.catch(reject);
        }
        // add main script tag if it's provided
        main && appendElement("script", { type: "module", src: main });
        resolve(swr.active!);
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
            waiting.onstatechange = () => run(true);
          }
        };
      }
    };

    // run the app immediately if there's an activated Service Worker
    run();
  });
}

export function fire({
  fetch: onFetch,
  waitPromise,
}: FireOptions = {}) {
  // @ts-expect-error missing types
  if (typeof clients === "undefined") {
    panic("Service Worker scope not found.");
  }

  let esmArchive: Archive | undefined;

  const on: typeof addEventListener = addEventListener;
  const bc = new BroadcastChannel(kSw);
  const cachePromise = caches.open(kSw);
  const queryEsmArchive = cachePromise.then((cache) =>
    cache.match("/+" + kTypeEsmArchive).then((res) =>
      res?.arrayBuffer().then((buf) => {
        try {
          esmArchive = new Archive(buf);
        } catch (err) {
          // ignore
        }
      })
    )
  );

  on("install", (evt) => {
    // @ts-expect-error `skipWaiting` is a global function in Service Worker
    skipWaiting();
    evt.waitUntil(waitPromise ? Promise.all([waitPromise, queryEsmArchive]) : queryEsmArchive);
  });

  on("activate", (evt) => {
    // @ts-expect-error `clients` is a global variable in Service Worker
    evt.waitUntil(clients.claim());
  });

  on("fetch", (evt) => {
    const { request } = evt as FetchEvent;
    const url = new URL(request.url);
    const { pathname } = url;
    const isSameOrigin = url.origin === location.origin;
    const pathOrHref = isSameOrigin ? pathname : request.url;
    const archive = esmArchive;
    const respondWith = (res: Response | Promise<Response>) => evt.respondWith(res);
    if (archive?.exists(pathOrHref)) {
      const file = archive.openFile(pathOrHref)!;
      respondWith(createResponse(file, { "content-type": file.type }));
    } else if (onFetch) {
      respondWith(onFetch(request));
    }
  });

  on("message", async (evt) => {
    const { data } = evt;
    if (Array.isArray(data)) {
      const [HEAD, buffer, gz] = data;
      if (HEAD === 0x127 && buffer instanceof ArrayBuffer) {
        try {
          let data = buffer;
          if (gz) {
            data = await createResponse(
              createResponse(buffer).body!.pipeThrough(new DecompressionStream("gzip")),
            ).arrayBuffer();
          }
          const archive = new Archive(data);
          const currentArchive = esmArchive;
          const stale = !currentArchive || archive.checksum !== currentArchive.checksum;
          if (stale) {
            esmArchive = archive;
            cachePromise.then((cache) => cache.put("/+" + kTypeEsmArchive, createResponse(data, { "content-type": kTypeEsmArchive })));
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

/** query all elements by the given selectors. */
function queryElement<T extends Element>(
  selectors: string,
  callback: (el: T) => void,
) {
  const el = doc!.querySelector<T>(selectors);
  if (el) {
    callback(el);
  }
}

/** get the attribute value of the given element. */
function getAttr(el: Element, name: string) {
  return el.getAttribute(name);
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

// run the `main` module if it's provided in the script tag with `src` attribute equals to current script url
// e.g. <script type="module" src="https://esm.sh/run" main="/main.mjs"></script>
doc && queryElement<HTMLScriptElement>("script[main]", (el) => {
  const src = el.src;
  const main = getAttr(el, "main");
  if (main && src && new URL(src, location.href).href === import.meta.url) {
    install({ main, swModule: getAttr(el, "sw") ?? undefined });
  }
});

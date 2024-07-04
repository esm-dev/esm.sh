/*! 🔥 esm.sh/run - speeding up your modern(es2015+) web application with service worker.
 *  Docs: https://docs.esm.sh/run
 */

import type { ArchiveEntry, RunOptions } from "./types/run.d.ts";

const global = globalThis;
const document: Document | undefined = global.document;
const clients: Clients | undefined = global.clients;
const modUrl = import.meta.url;
const kRun = "esm.sh/run";
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

export async function run({
  main,
  onUpdateFound = () => location.reload(),
  swModule,
  swScope,
  swUpdateViaCache,
}: RunOptions = {}): Promise<ServiceWorker> {
  const serviceWorker = navigator.serviceWorker;
  const hasController = serviceWorker.controller !== null;

  const reg = await serviceWorker.register(swModule ?? "/sw.js", {
    type: "module",
    scope: swScope,
    updateViaCache: swUpdateViaCache,
  });

  return new Promise<ServiceWorker>(async (resolve, reject) => {
    const run = async () => {
      if (reg.active?.state === "activated") {
        let dl: Promise<boolean> | undefined;
        // download esm-archive and and send it to the Service Worker if has any
        queryElement<HTMLLinkElement>(`link[type^="${kTypeEsmArchive}"][href]`, (el) => {
          dl = fetch(el.href).then((res) => {
            if (!res.ok) {
              throw new Error("Failed to download esm-archive: " + (res.statusText ?? res.status));
            }
            return res.arrayBuffer();
          }).then(async (arrayBuffer) => {
            const checksum = attr(el, "checksum");
            if (checksum) {
              const buf = await crypto.subtle.digest("SHA-256", arrayBuffer);
              if (btoa(String.fromCharCode(...new Uint8Array(buf))) !== checksum) {
                throw new Error("Invalid esm-archive: the checksum does not match");
              }
            }
            return new Promise<boolean>((res, rej) => {
              new BroadcastChannel(kRun).onmessage = ({ data }) => {
                if (data === 0) {
                  rej(new Error("Failed to load esm-archive"));
                } else {
                  res(data === 2);
                }
              };
              reg.active!.postMessage([0x7f, arrayBuffer, el.type.endsWith("+gzip")]);
            });
          });
        });
        if (dl) {
          if (hasController) {
            dl.then((isStale) => isStale && onUpdateFound());
          } else {
            // if there's no controller, wait for the esm-archive to be loaded
            await dl.catch(reject);
          }
        }
        // add main script tag if it's provided
        if (main) {
          import(main);
        }
        resolve(reg.active!);
      }
    };

    // detect Service Worker install/update available and wait for it to become installed
    reg.onupdatefound = () => {
      const installing = reg.installing;
      if (installing) {
        installing.onerror = (e) => reject(e.error);
        installing.onstatechange = () => {
          const waiting = reg.waiting;
          if (waiting) {
            waiting.onstatechange = hasController ? onUpdateFound : run;
          }
        };
      }
    };

    // run the app immediately if the Service Worker is already installed
    if (hasController) {
      run();
    }
  });
}

function fire() {
  // esm-archive bundles modules specified in the import map
  let esmArchive: Archive | undefined;

  const on: typeof addEventListener = addEventListener;
  const bc = new BroadcastChannel(kRun);
  const cachePromise = caches.open(kRun);

  on("install", (evt) => {
    // @ts-expect-error `skipWaiting` is a global function in Service Worker
    skipWaiting();
    // query the esm-archive from cache and load it into memory if exists
    evt.waitUntil(cachePromise.then((cache) =>
      cache.match("/+" + kTypeEsmArchive).then((res) =>
        res?.arrayBuffer().then((buf) => {
          esmArchive = new Archive(buf);
        })
      )
    ));
  });

  on("activate", (evt) => {
    // When a service worker is initially registered, pages won't use it until they next load.
    // The `clients.claim()` method causes those pages to be controlled immediately.
    evt.waitUntil(clients!.claim());
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
    }
  });

  on("message", async (evt) => {
    const { data } = evt;
    if (Array.isArray(data)) {
      const [HEAD, buffer, gz] = data;
      if (HEAD === 0x7f && buffer instanceof ArrayBuffer) {
        try {
          let data = buffer;
          if (gz) {
            data = await createResponse(
              createResponse(buffer).body!.pipeThrough(new DecompressionStream("gzip")),
            ).arrayBuffer();
          }
          const archive = new Archive(data);
          const isStale = !esmArchive || archive.checksum !== esmArchive.checksum;
          if (isStale) {
            esmArchive = archive;
            cachePromise.then((cache) => cache.put("/+" + kTypeEsmArchive, createResponse(data, { "content-type": kTypeEsmArchive })));
          }
          bc.postMessage(isStale ? 2 : 1);
        } catch (err) {
          bc.postMessage(0);
          console.error(err);
        }
      }
    }
  });
}

/** query the element with the given selector and run the callback if found. */
function queryElement<T extends Element>(
  selector: string,
  callback: (el: T) => void,
) {
  const el = document!.querySelector<T>(selector);
  if (el) {
    callback(el);
  }
}

/** get the attribute value of the given element. */
function attr(el: Element, name: string) {
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

if (document) {
  // run the `main` module if it's provided in the script tag with `src` attribute equals to current script url
  // e.g. <script type="module" src="https://esm.sh/run" main="/main.mjs"></script>
  queryElement<HTMLScriptElement>("script[type='module'][src][main]", (el) => {
    const src = el.src;
    const main = attr(el, "main");
    if (src && main && new URL(src, location.href).href === modUrl) {
      run({ main, swModule: attr(el, "sw") ?? undefined });
    }
  });
  // compatibility with esm.sh/run (v1) which has been renamed to esm.sh/tsx
  queryElement<HTMLScriptElement>("script[type^='text/']", () => {
    import(new URL("/tsx", modUrl).href);
  });
} else if (clients) {
  fire();
}

export default run;

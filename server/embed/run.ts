/*! 🔥 esm.sh/run - speeding up your modern(es2015+) web application with service worker.
 *  Docs: https://docs.esm.sh/run
 */

import type { RunOptions, VFile } from "./types/run.d.ts";

const global = globalThis;
const document: Document | undefined = global.document;
const clients: Clients | undefined = global.clients;
const kRun = "esm.sh/run";
const kVFSdbStoreName = "files";

class VFS {
  private _db: Promise<IDBDatabase> | IDBDatabase;

  constructor() {
    this._db = this._openDB();
  }

  private _openDB(): Promise<IDBDatabase> {
    const req = indexedDB.open(kRun);
    req.onupgradeneeded = () => {
      const db = req.result;
      if (!db.objectStoreNames.contains(kVFSdbStoreName)) {
        db.createObjectStore(kVFSdbStoreName, { keyPath: "url" });
      }
    };
    return promisifyIDBRequest<IDBDatabase>(req).then((db) => {
      // reopen the db on 'close' event
      db.onclose = () => {
        this._db = this._openDB();
      };
      return this._db = db;
    });
  }

  private async _tx(readonly = false) {
    const db = await this._db;
    return db.transaction(kVFSdbStoreName, readonly ? "readonly" : "readwrite").objectStore(kVFSdbStoreName);
  }

  async readFile(name: string | URL): Promise<Uint8Array | null> {
    const db = await this._tx(true);
    const ret = await promisifyIDBRequest<VFile | undefined>(db.get(normalizeUrl(name)));
    return ret?.content ?? null;
  }

  async writeFile(
    url: string | URL,
    content: Uint8Array,
    options?: { contentType?: string; lastModified?: number },
  ): Promise<void> {
    const db = await this._tx();
    const file: VFile = {
      ...options,
      url: normalizeUrl(url),
      content,
    };
    return promisifyIDBRequest(db.put(file));
  }
}

export async function run({
  main,
  onUpdateFound = () => location.reload(),
  swModule,
  swScope,
}: RunOptions = {}): Promise<ServiceWorker> {
  const serviceWorker = navigator.serviceWorker;
  const hasController = serviceWorker.controller !== null;

  const reg = await serviceWorker.register(swModule ?? "/sw.js", {
    type: "module",
    scope: swScope,
  });

  return new Promise<ServiceWorker>(async (resolve, reject) => {
    const run = async () => {
      if (reg.active?.state === "activated") {
        let dl: Promise<boolean> | undefined;
        queryElement<HTMLLinkElement>("link[rel='preload'][as='fetch'][type='application/esm-bundle'][href]", (el) => {
          dl = fetch(el.href).then((res) => {
            if (!res.ok) {
              throw new Error("Failed to download esm-bundle: " + (res.statusText ?? res.status));
            }
            return res.arrayBuffer();
          }).then(async (arrayBuffer) => {
            const checksumAttr = attr(el, "checksum");
            if (checksumAttr && await shasum(arrayBuffer) !== checksumAttr) {
              throw new Error("Invalid esm-bundle: the checksum does not match");
            }
            return new Promise<boolean>((res, rej) => {
              new BroadcastChannel(kRun).onmessage = ({ data }) => {
                if (data === 0) {
                  rej(new Error("Failed to load esm-bundle"));
                } else {
                  res(data === 2);
                }
              };
              reg.active!.postMessage([0x7f, arrayBuffer]);
            });
          });
        });
        if (dl) {
          if (hasController) {
            dl.then((isStale) => isStale && onUpdateFound());
          } else {
            // if there's no controller, wait for the esm-bundle to be loaded
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
  const on: typeof addEventListener = addEventListener;
  const bc = new BroadcastChannel(kRun);
  const vfs = new VFS();
  const esmBundleSavePath = ".esm-bundle.json";

  let imports: Record<string, string> | undefined;
  const loadImportsFromVFS = async () => {
    const jsonContent = await vfs.readFile(esmBundleSavePath);
    if (jsonContent) {
      imports = await parseImports(jsonContent.buffer);
    }
  };
  const parseImports = async (jsonContent: ArrayBuffer): Promise<Record<string, string>> => {
    const v = JSON.parse(new TextDecoder().decode(jsonContent!));
    if (typeof v !== "object" || v === null) {
      throw new Error("Invalid esm-bundle: the content is not an object");
    }
    v.$checksum = await shasum(jsonContent!);
    return v;
  };

  on("install", (evt) => {
    // @ts-expect-error `skipWaiting` is a global function in Service Worker
    skipWaiting();
    // query the esm-bundle from cache and load it into memory if exists
    evt.waitUntil(loadImportsFromVFS());
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
    if (imports && pathOrHref in imports) {
      evt.respondWith(createResponse(imports[pathOrHref], { "content-type": "application/javascript; charset=utf-8" }));
    }
  });

  on("message", async (evt) => {
    const { data } = evt;
    if (Array.isArray(data)) {
      const [HEAD, buffer] = data;
      if (HEAD === 0x7f && buffer instanceof ArrayBuffer) {
        try {
          const newImports = await parseImports(buffer);
          const isStale = !imports || imports.$checksum !== newImports.$checksum;
          if (isStale) {
            imports = newImports;
            vfs.writeFile(esmBundleSavePath, new Uint8Array(buffer));
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

/** get the attribute value of the given element. */
function attr(el: Element, name: string) {
  return el.getAttribute(name);
}

/** query the element with the given selector and run the callback if found. */
function queryElement<T extends Element>(selector: string, callback: (el: T) => void) {
  const el = document!.querySelector<T>(selector);
  if (el) {
    callback(el);
  }
}

/** create a response object. */
function createResponse(body: BodyInit | null, headers?: HeadersInit, status?: number): Response {
  return new Response(body, { headers, status });
}

/** promisify the given IDBRequest. */
function promisifyIDBRequest<T>(req: IDBRequest): Promise<T> {
  return new Promise((resolve, reject) => {
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

/** normalize the given URL string or URL object. */
function normalizeUrl(url: string | URL) {
  return (typeof url === "string" ? new URL(url, "file:///") : url).href;
}

async function shasum(input: ArrayBuffer): Promise<string> {
  const buf = await crypto.subtle.digest("SHA-256", input);
  return btoa(String.fromCharCode(...new Uint8Array(buf)));
}

if (document) {
  // run the `main` module if it's provided in the script tag with `src` attribute equals to current script url
  // e.g. <script type="module" src="https://esm.sh/run" main="/main.mjs" sw="/sw.mjs"></script>
  queryElement<HTMLScriptElement>("script[type='module'][src][main]", (el) => {
    const src = el.src;
    const main = attr(el, "main");
    if (src === import.meta.url && main) {
      run({ main, swModule: attr(el, "sw") ?? undefined });
    }
  });
  // compatibility with esm.sh/run (v1) which has been renamed to esm.sh/tsx
  queryElement<HTMLScriptElement>("script[type^='text/']", () => {
    import("https://esm.sh/tsx");
  });
} else if (clients) {
  fire();
}

export default run;

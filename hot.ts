/*! 🔥 esm.sh/hot
 *  Docs: https://til.esm.sh/hot
 */

/// <reference lib="webworker" />

import type { FetchHandler, HotCore, ImportMap, IncomingTest, Plugin, VFile } from "./server/embed/types/hot.d.ts";

const VERSION = 135;
const doc: Document | undefined = globalThis.document;
const loc = location;
const parse = JSON.parse;
const localhosts = new Set(["localhost", "127.0.0.1"]);
const kHot = "esm.sh/hot";
const kMessage = "message";
const kVfs = "vfs";

/** A virtual file system using indexed database. */
class VFS {
  #dbPromise: Promise<IDBDatabase>;

  constructor(scope: string, version: number) {
    const req = indexedDB.open(scope, version);
    req.onupgradeneeded = () => {
      const db = req.result;
      if (!db.objectStoreNames.contains(kVfs)) {
        db.createObjectStore(kVfs, { keyPath: "name" });
      }
    };
    this.#dbPromise = waitIDBRequest<IDBDatabase>(req);
  }

  async #begin(readonly = false) {
    const db = await this.#dbPromise;
    return db.transaction(kVfs, readonly ? "readonly" : "readwrite")
      .objectStore(kVfs);
  }

  async get(name: string) {
    const tx = await this.#begin(true);
    return waitIDBRequest<VFile | undefined>(tx.get(name));
  }

  async put(name: string, data: VFile["data"], meta?: VFile["meta"]) {
    const record: VFile = { name, data };
    if (meta) {
      record.meta = meta;
    }
    const tx = await this.#begin();
    return waitIDBRequest<string>(tx.put(record));
  }

  async delete(name: string) {
    const tx = await this.#begin();
    return waitIDBRequest<void>(tx.delete(name));
  }
}

/** Hot class implements the `HotCore` interface. */
class Hot implements HotCore {
  #cache: Cache | null = null;
  #importMap: Required<ImportMap> | null = null;
  #fetchListeners: { test: IncomingTest | RegExp; handler: FetchHandler }[] = [];
  #fireListeners: ((sw: ServiceWorker) => void)[] = [];
  #isDev = localhosts.has(location.hostname);
  #promises: Promise<any>[] = [];
  #vfs = new VFS(kHot, VERSION);
  #registeredSW: URL | null = null;
  #activatedSW: ServiceWorker | null = null;

  get cache() {
    return this.#cache ?? (this.#cache = createCacheProxy(kHot + VERSION));
  }

  get importMap() {
    return this.#importMap ?? (this.#importMap = parseImportMap());
  }

  get isDev() {
    return this.#isDev;
  }

  get vfs() {
    return this.#vfs;
  }

  onFetch(test: IncomingTest | RegExp, handler: FetchHandler) {
    if (!doc) {
      this.#fetchListeners.push({ test, handler });
    }
    return this;
  }

  onFire(handler: (reg: ServiceWorker) => void) {
    if (doc) {
      if (this.#activatedSW) {
        handler(this.#activatedSW);
      } else {
        this.#fireListeners.push(handler);
      }
    }
    return this;
  }

  waitUntil(promise: Promise<void>) {
    this.#promises.push(promise);
  }

  async fire(swScript = "/sw.js") {
    const sw = navigator.serviceWorker;
    if (!sw) {
      throw new Error("Service Worker not supported.");
    }

    if (this.#registeredSW) {
      return;
    }
    this.#registeredSW = new URL(swScript, loc.href);

    const reg = await sw.register(this.#registeredSW, {
      type: "module",
      updateViaCache: this.#isDev ? undefined : "all",
    });

    let isFirstInstalled = false;

    // detect Service Worker update available and wait for it to become installed
    reg.onupdatefound = () => {
      const { installing } = reg;
      if (installing) {
        installing.onstatechange = () => {
          const { waiting } = reg;
          if (waiting && !sw.controller) {
            waiting.onstatechange = () => {
              if (reg.active?.state === "activated") {
                isFirstInstalled = true;
                this.#fireApp(reg.active);
              }
            };
          }
        };
      }
    };

    // detect controller change and refresh the page
    sw.oncontrollerchange, () => {
      !isFirstInstalled && loc.reload();
    };

    // fire immediately if there's an active Service Worker
    if (reg.active) {
      this.#fireApp(reg.active);
    }
  }

  async #fireApp(sw: ServiceWorker) {
    // update importmap in Service Worker
    sw.postMessage({ IMPORT_MAP: this.importMap });

    // wait until all promises resolved
    await Promise.all(this.#promises);

    // fire all `fire` listeners
    for (const handler of this.#fireListeners) {
      handler(sw);
    }
    this.#activatedSW = sw;

    // apply "[type=hot/module]" script tags
    queryElements<HTMLScriptElement>("script[type='hot/module']", (el) => {
      const copy = el.cloneNode(true) as HTMLScriptElement;
      copy.type = "module";
      el.replaceWith(copy);
    });
  }

  use(...plugins: Plugin[]) {
    plugins.forEach((plugin) => plugin.setup(this));
    return this;
  }

  listen() {
    // @ts-expect-error missing dts
    if (typeof clients === "undefined") {
      throw new Error("Service Worker scope not found.");
    }

    const vfs = this.#vfs;
    const serveVFS = async (name: string) => {
      const file = await vfs.get(name);
      if (!file) {
        return createResponse("Not Found", {}, 404);
      }
      const headers: HeadersInit = {
        "content-type": file.meta?.contentType ?? "binary/octet-stream",
      };
      return createResponse(file.data, headers);
    };
    const on: typeof addEventListener = addEventListener;

    on("install", (evt) => {
      // @ts-expect-error missing dts
      skipWaiting();
      evt.waitUntil(Promise.all(this.#promises));
    });

    on("activate", (evt) => {
      // @ts-expect-error missing dts
      evt.waitUntil(clients.claim());
    });

    on("fetch", (evt) => {
      const { request } = evt;
      const respondWith = evt.respondWith.bind(evt);
      const url = new URL(request.url);
      const { pathname } = url;
      const fetchListeners = this.#fetchListeners;
      if (fetchListeners.length > 0) {
        for (const { test, handler } of fetchListeners) {
          if (test instanceof RegExp ? test.test(url.pathname) : test(url, request.method, request.headers)) {
            return respondWith(handler(request));
          }
        }
      }
      if (isSameOrigin(url) && pathname.startsWith("/@hot/")) {
        respondWith(serveVFS(pathname.slice(6)));
      }
    });

    // listen to `message` event for importmap updating
    on(kMessage, (evt) => {
      const { data } = evt;
      if (isObject(data)) {
        const im = data.IMPORT_MAP;
        if (isObject(im)) {
          this.#importMap = im as Required<ImportMap>;
        }
      }
    });
  }
}

/** query all elements by the given selectors. */
function queryElements<T extends Element>(
  selectors: string,
  callback: (value: T) => void,
) {
  // @ts-expect-error throw error if document is not available
  doc.querySelectorAll(selectors).forEach(callback);
}

/** parse importmap from <script> with `type=importmap` */
function parseImportMap() {
  const importMap: Required<ImportMap> = {
    $support: HTMLScriptElement.supports?.("importmap"),
    imports: {},
    scopes: {},
  };
  if (!doc) {
    return importMap;
  }
  const script = doc.querySelector("script[type=importmap]");
  if (script) {
    try {
      const json = parse(script.textContent!);
      if (isObject(json)) {
        const { imports, scopes } = json;
        if (isObject(imports)) {
          validateImports(imports);
          importMap.imports = imports;
        }
        if (isObject(scopes)) {
          validateScopes(scopes);
          importMap.scopes = scopes;
        }
      }
    } catch (err) {
      console.error("Invalid importmap", err[kMessage]);
    }
  }
  return importMap;
}

function validateImports(imports: Record<string, unknown>) {
  for (const [k, v] of Object.entries(imports)) {
    if (!v || typeof v !== "string") {
      delete imports[k];
    }
  }
}

function validateScopes(imports: Record<string, unknown>) {
  for (const [k, v] of Object.entries(imports)) {
    if (isObject(v)) {
      validateImports(v);
    } else {
      delete imports[k];
    }
  }
}

/** create a cache proxy object. */
function createCacheProxy(cacheName: string) {
  const cachePromise = caches.open(cacheName);
  return new Proxy({}, {
    get: (_, name) => async (...args: unknown[]) => {
      return (await cachePromise as any)[name](...args);
    },
  }) as Cache;
}

/** create a response object. */
function createResponse(
  body: BodyInit | null,
  headers: HeadersInit = {},
  status = 200,
): Response {
  return new Response(body, { headers, status });
}

/** check if the given value is an object. */
function isObject(v: unknown): v is Record<string, any> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

/** check if the given url has the same origin with current loc. */
function isSameOrigin(url: URL) {
  return url.origin === loc.origin;
}

/** wait for the given IDBRequest. */
function waitIDBRequest<T>(req: IDBRequest): Promise<T> {
  return new Promise((resolve, reject) => {
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

export const hot = new Hot();
export default hot;

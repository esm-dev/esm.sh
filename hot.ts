/*! 🔥 esm.sh/hot
*
* Docs: https://esm.sh/hot/docs
*
*/

/// <reference lib="dom" />
/// <reference lib="webworker" />

import type {
  FetchHandler,
  HotCore,
  ImportMap,
  Loader,
  Plugin,
  URLTest,
  VFSRecord,
} from "./server/embed/types/hot.d.ts";

const VERSION = 135;
const plugins: Plugin[] = [];
const doc = globalThis.document;
const enc = new TextEncoder();
const kSkipWaiting = "SKIP_WAITING";
const kVfs = "vfs";
const kContentType = "content-type";
const kContentSource = "x-content-source";
const kImportmapJson = "internal:importmap.json";

/** virtual file system using indexed database */
class VFS {
  #dbPromise: Promise<IDBDatabase>;

  constructor() {
    // open indexed database
    const req = indexedDB.open("esm.sh/hot", VERSION);
    req.onupgradeneeded = () => {
      const db = req.result;
      if (!db.objectStoreNames.contains(kVfs)) {
        db.createObjectStore(kVfs, { keyPath: "name" });
      }
    };
    this.#dbPromise = waitIDBRequest<IDBDatabase>(req);
  }

  async #start(readonly = false) {
    const db = await this.#dbPromise;
    return db.transaction(kVfs, readonly ? "readonly" : "readwrite")
      .objectStore(kVfs);
  }

  async get(name: string) {
    const tx = await this.#start(true);
    return waitIDBRequest<VFSRecord | undefined>(tx.get(name));
  }

  async put(name: string, data: VFSRecord["data"], meta?: VFSRecord["meta"]) {
    const record: VFSRecord = { name, data };
    if (meta) {
      record.meta = meta;
    }
    const tx = await this.#start();
    return waitIDBRequest<string>(tx.put(record));
  }

  async delete(name: string) {
    const tx = await this.#start();
    return waitIDBRequest<void>(tx.delete(name));
  }
}

/** 🔥 class implements HotCore interface. */
class Hot implements HotCore {
  #basePath = new URL(".", location.href).pathname;
  #cache: Cache | null = null;
  #importMap: Required<ImportMap> | null = null;
  #fetchListeners: { test: URLTest; handler: FetchHandler }[] = [];
  #fireListeners: ((sw: ServiceWorker) => void)[] = [];
  #isDev = isLocalhost(location);
  #loaders: Loader[] = [];
  #promises: Promise<any>[] = [];
  #vfs = new VFS();
  #fired = false;

  constructor(plugins: Plugin[] = []) {
    plugins.forEach((plugin) => plugin.setup(this));
  }

  get basePath() {
    return this.#basePath;
  }

  get cache() {
    return this.#cache ??
      (this.#cache = crateCacheProxy("esm.sh/hot/v" + VERSION));
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

  onFetch(test: URLTest, handler: FetchHandler) {
    if (!doc) {
      this.#fetchListeners.push({ test, handler });
    }
    return this;
  }

  onFire(handler: (reg: ServiceWorker) => void) {
    if (doc) {
      this.#fireListeners.push(handler);
    }
    return this;
  }

  onLoad(
    test: RegExp,
    load: Loader["load"],
    fetch?: Loader["fetch"],
    priority?: string,
  ) {
    if (!doc) {
      if (priority === "eager") {
        this.#loaders.unshift({ test, load, fetch });
      } else {
        this.#loaders.push({ test, load, fetch });
      }
    }
    return this;
  }

  waitUntil(promise: Promise<void>) {
    if (!this.#promises.includes(promise)) {
      this.#promises.push(promise);
    }
  }

  async fire(swScript = "/sw.js") {
    if (!doc) {
      throw new Error("Document not found.");
    }

    const sw = navigator.serviceWorker;
    if (!sw) {
      throw new Error("Service Worker not supported.");
    }

    if (this.#fired) {
      console.warn("Got multiple fire() calls, ignored.");
      return;
    }

    const swScriptUrl = new URL(swScript, location.href);
    this.#basePath = new URL(".", swScriptUrl).pathname;
    this.#fired = true;

    const reg = await sw.register(swScriptUrl, {
      type: "module",
      updateViaCache: this.isDev ? "none" : "all",
    });
    let refreshing = false;

    // detect Service Worker update available and wait for it to become installed
    reg.addEventListener("updatefound", () => {
      reg.installing?.addEventListener("statechange", () => {
        const { waiting } = reg;
        if (waiting) {
          // if there's an existing controller (previous Service Worker)
          if (sw.controller) {
            // ask user to confirm update?
            waiting.postMessage(kSkipWaiting);
          } else {
            // otherwise it's the first install
            waiting.postMessage(kSkipWaiting);
            waiting.addEventListener("statechange", () => {
              if (reg.active && !refreshing) {
                refreshing = true;
                this.#fireApp(reg.active, true);
                this.isDev && console.log("🔥 app registered.");
              }
            });
          }
        }
      });
    });

    // detect controller change and refresh the page
    sw.addEventListener("controllerchange", () => {
      !refreshing && location.reload();
    });

    // if there's a waiting, send skip waiting message
    reg.waiting?.postMessage(kSkipWaiting);

    // fire immediately if there's an active Service Worker
    if (reg.active) {
      this.#fireApp(reg.active);
    }
  }

  async #fireApp(sw: ServiceWorker, firstInstall = false) {
    const isDev = this.#isDev;
    if (isDev) {
      const url = "./hot/dev";
      const { setup } = await import(url);
      setup(this);
    }
    this.#promises.push(this.#vfs.put(kImportmapJson, this.importMap as any));
    await Promise.all(this.#promises);
    for (const onFire of this.#fireListeners) {
      onFire(sw);
    }
    if (firstInstall) {
      doc.querySelectorAll("link[rel=stylesheet]").forEach((el) => {
        const href = el.getAttribute("href");
        if (href) {
          const url = new URL(href, location.href);
          if (isSameOrigin(url)) {
            url.searchParams.set("t", getTimeStamp());
            el.setAttribute("href", url.pathname + url.search);
          }
        }
      });
    }
    doc.querySelectorAll("script").forEach((el) => {
      if (el.type === "text/babel" || el.type === "hot/module") {
        const copy = el.cloneNode(true) as HTMLScriptElement;
        copy.type = "module";
        el.replaceWith(copy);
      }
    });
    customElements.define(
      "hot-html",
      class HotHtml extends HTMLElement {
        connectedCallback() {
          const src = this.getAttribute("src");
          if (!src) {
            return;
          }
          const url = new URL(src, location.href);
          const root = this.hasAttribute("shadow")
            ? this.attachShadow({ mode: "open" })
            : this;
          root.innerHTML = "<slot></slot>";
          const load = async (first?: boolean) => {
            if (!first) {
              url.searchParams.set("t", getTimeStamp());
            }
            const res = await fetch(url);
            if (res.ok) {
              const tpl = document.createElement("template");
              tpl.innerHTML = await res.text();
              root.replaceChildren(tpl.content);
            }
          };
          if (isDev && isSameOrigin(url)) {
            __hot_hmr_callbacks.add(url.pathname, load);
          }
          load(true);
        }
      },
    );
    this.isDev && console.log("🔥 app fired.");
  }

  listen() {
    // @ts-ignore clients
    if (typeof clients === "undefined") {
      throw new Error("Service Worker scope not found.");
    }

    const mimeTypes: Record<string, string[]> = {
      "a/javascript;": ["js", "mjs"],
      "a/json;": ["json", "map"],
      "a/wasm": ["wasm"],
      "a/xml;": ["xml"],
      "i/gif": ["gif"],
      "i/jpeg": ["jpeg", "jpg"],
      "i/png": ["png"],
      "i/svg+xml;": ["svg"],
      "i/webp": ["webp"],
      "t/css;": ["css"],
      "t/html;": ["html", "htm"],
      "t/plain;": ["csv", "md", "txt"],
    };
    const alias: Record<string, string> = {
      a: "application",
      i: "image",
      t: "text",
    };
    const typesMap = new Map<string, string>();
    for (const mimeType in mimeTypes) {
      for (const ext of mimeTypes[mimeType]) {
        const endsWithSemicolon = mimeType.endsWith(";");
        let suffix = mimeType.slice(1);
        if (endsWithSemicolon) {
          suffix += " charset=utf-8";
        }
        typesMap.set(ext, alias[mimeType.charAt(0)] + suffix);
      }
    }

    const vfs = this.#vfs;
    const serveVFS = async (name: string) => {
      const file = await vfs.get(name);
      if (!file) {
        return new Response("Not Found", { status: 404 });
      }
      const headers: HeadersInit = [[
        kContentType,
        file.meta?.contentType ?? typesMap.get(getExtname(name)) ??
          "binary/octet-stream",
      ]];
      return new Response(file.data, { headers });
    };

    const loaderHeaders = (contentType?: string) => {
      const headers = new Headers();
      headers.set(kContentType, contentType ?? typesMap.get("js")!);
      headers.set(kContentSource, "hot-loader");
      return headers;
    };
    const serveLoader = async (loader: Loader, url: URL, req: Request) => {
      const res = await (loader.fetch ?? fetch)(req);
      if (!res.ok || res.headers.get(kContentSource) === "hot-loader") {
        return res;
      }
      const resHeaders = res.headers;
      let etag = resHeaders.get("etag");
      if (!etag) {
        const size = resHeaders.get("content-length");
        const modtime = resHeaders.get("last-modified");
        if (size && modtime) {
          etag = etag = "W/" + size + "-" + modtime;
        }
      }
      let buffer: string | null = null;
      const source = async () => {
        if (buffer === null) {
          buffer = await res.text();
        }
        return buffer;
      };
      let cacheKey = url.href;
      if (url.host === location.host) {
        url.searchParams.delete("t");
        cacheKey = url.pathname.slice(1) + url.search.replace(/=(&|$)/g, "");
      }
      let isDev = this.#isDev;
      if (req.headers.get("hot-loader-env") === "production") {
        isDev = false;
      }
      cacheKey = "loader" + (isDev ? "(dev)" : "") + ":" + cacheKey;
      const [vfsImportMap, cached] = await Promise.all([
        vfs.get(kImportmapJson),
        vfs.get(cacheKey),
      ]);
      const importMap: ImportMap = (vfsImportMap?.data as unknown) ?? {};
      const checksum = await computeHash(
        enc.encode(JSON.stringify(importMap) + (etag ?? await source())),
      );
      if (cached && cached.meta?.checksum === checksum) {
        if (!res.bodyUsed) {
          res.body?.cancel();
        }
        const headers = loaderHeaders(cached.meta?.contentType);
        headers.set("hot-loader-cache-status", "HIT");
        return new Response(cached.data, { headers });
      }
      try {
        const options = { isDev, importMap };
        const ret = await loader.load(url, await source(), options);
        const { code, contentType, deps, map } = ret;
        let body = code;
        if (map) {
          body +=
            "\n//# sourceMappingURL=data:application/json;charset=utf-8;base64,";
          body += btoa(map);
        }
        vfs.put(cacheKey, body, { checksum, contentType, deps });
        return new Response(body, { headers: loaderHeaders(contentType) });
      } catch (err) {
        console.error(err);
        return new Response(err.message, { status: 500 });
      }
    };
    const fetchWithCache = async (req: Request) => {
      const cache = this.cache;
      const res = await cache.match(req);
      if (res) return res;
      const r = await fetch(req.url);
      if (r.status !== 200) return r;
      await cache.put(req, r.clone());
      return r;
    };

    // @ts-ignore disable type check
    self.oninstall = (evt) => evt.waitUntil(Promise.all(this.#promises));

    // @ts-ignore disable type check
    self.onactivate = (evt) => evt.waitUntil(clients.claim());

    // @ts-ignore disable type check
    self.onfetch = (evt: FetchEvent) => {
      const { request } = evt;
      const url = new URL(request.url);
      const { pathname, hostname } = url;
      const loaders = this.#loaders;
      const fetchListeners = this.#fetchListeners;
      if (fetchListeners.length > 0) {
        for (const { test, handler } of fetchListeners) {
          if (test(url, request)) {
            return evt.respondWith(handler(request));
          }
        }
      }
      if (
        url.hostname === "esm.sh" &&
        /\w@\d+.\d+\.\d+(-|\/|\?|$)/.test(url.pathname)
      ) {
        return evt.respondWith(fetchWithCache(request));
      }
      if (hostname === location.hostname) {
        if (pathname.startsWith("/@hot/")) {
          evt.respondWith(serveVFS(pathname.slice(1)));
        } else if (
          url.pathname !== location.pathname &&
          !url.searchParams.has("raw")
        ) {
          const loader = loaders.find(({ test }) => test.test(pathname));
          if (loader) {
            evt.respondWith(serveLoader(loader, url, request));
          }
        }
      }
    };

    self.onmessage = (evt) => {
      if (evt.data === kSkipWaiting) {
        // @ts-ignore skipWaiting
        self.skipWaiting();
      }
    };
  }
}

/** parse importmap from <script> with `type=importmap` */
function parseImportMap() {
  if (!doc) {
    throw new Error("Document not found.");
  }
  const importMap: Required<ImportMap> = {
    $support: HTMLScriptElement.supports?.("importmap"),
    imports: {},
    scopes: {},
  };
  const script = doc.querySelector("head>script[type=importmap]");
  if (script) {
    try {
      const v = JSON.parse(script.textContent!);
      if (isObject(v)) {
        const { imports, scopes } = v;
        for (const k in imports) {
          importMap.imports[k] = imports[k];
        }
        if (isObject(scopes)) {
          importMap.scopes = scopes;
        }
      }
    } catch (err) {
      console.error("Failed to parse importmap:", err);
    }
  }
  return importMap;
}

/** create a cache proxy object. */
function crateCacheProxy(cacheName: string) {
  const cachePromise = caches.open(cacheName);
  return new Proxy({}, {
    get: (_, name) => async (...args: unknown[]) => {
      return (await cachePromise as any)[name](...args);
    },
  }) as Cache;
}

/** check if the given url has the same origin with current location. */
function isSameOrigin(url: URL) {
  return url.origin === location.origin;
}

/** check if the given value is an object. */
function isObject(v: unknown) {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

/** check if the url is localhost. */
function isLocalhost({ hostname }: URL | Location) {
  return hostname === "localhost" || hostname === "127.0.0.1";
}

/** get a timestamp string. */
function getTimeStamp() {
  return Date.now().toString(36);
}

/** get the extension name of the given path. */
function getExtname(path: string): string {
  const i = path.lastIndexOf(".");
  if (i >= 0) {
    return path.slice(i + 1);
  }
  return "";
}

/** wait for the given IDBRequest. */
function waitIDBRequest<T>(req: IDBRequest): Promise<T> {
  return new Promise((resolve, reject) => {
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

/** compute the hash of the given input, default algorithm is SHA-1. */
async function computeHash(
  input: Uint8Array,
  algorithm: AlgorithmIdentifier = "SHA-1",
): Promise<string> {
  const buffer = new Uint8Array(await crypto.subtle.digest(algorithm, input));
  return [...buffer].map((b) => b.toString(16).padStart(2, "0")).join("");
}

// 🔥
export default new Hot(plugins);

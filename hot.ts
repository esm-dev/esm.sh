/*! 🔥 esm.sh/hot
*
* Docs: https://esm.sh/hot/docs
*
*/

/// <reference lib="dom" />
/// <reference lib="webworker" />

interface Plugin {
  name: string;
  setup: (hot: Hot) => void;
}

interface Loader {
  test: RegExp;
  load: (url: URL, source: string, options: LoadOptions) => Promise<{
    code: string;
    contentType?: string;
    map?: string;
  }>;
}

interface LoadOptions {
  isDev: boolean;
  importMap: ImportMap;
}

interface ImportMap {
  $support?: boolean;
  imports?: Record<string, string>;
  scopes?: Record<string, Record<string, string>>;
}

interface FetchHandler {
  (req: Request): Response | Promise<Response>;
}

interface URLTest {
  (url: URL, req: Request): boolean;
}

interface VFSRecord {
  name: string;
  data: string | Uint8Array;
  meta?: {
    checksum?: string;
    contentType?: string;
  };
}

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
    let onOpen: (db: IDBDatabase) => void;
    let onError: (reason: DOMException | null) => void;
    this.#dbPromise = new Promise<IDBDatabase>((resolve, reject) => {
      onOpen = resolve;
      onError = reject;
    });

    // open indexed database
    const openRequest = indexedDB.open("esm.sh/hot", VERSION);
    openRequest.onupgradeneeded = function () {
      const db = openRequest.result;
      if (!db.objectStoreNames.contains(kVfs)) {
        db.createObjectStore(kVfs, { keyPath: "name" });
      }
    };
    openRequest.onsuccess = function () {
      onOpen(openRequest.result);
    };
    openRequest.onerror = function () {
      onError(openRequest.error);
    };
  }

  async #getDbStore(mode: IDBTransactionMode) {
    const db = await this.#dbPromise;
    return db.transaction(kVfs, mode).objectStore(kVfs);
  }

  async get(name: string) {
    const store = await this.#getDbStore("readonly");
    const req = store.get(name);
    return new Promise<VFSRecord | null>(
      (resolve, reject) => {
        req.onsuccess = () => resolve(req.result ? req.result : null);
        req.onerror = () => reject(req.error);
      },
    );
  }

  async put(
    name: string,
    data: string | Uint8Array,
    meta?: VFSRecord["meta"],
  ) {
    const store = await this.#getDbStore("readwrite");
    const record: VFSRecord = { name, data };
    if (meta) {
      record.meta = meta;
    }
    const req = store.put(record);
    return new Promise<void>((resolve, reject) => {
      req.onsuccess = () => resolve();
      req.onerror = () => reject(req.error);
    });
  }
}

/** 🔥 class */
class Hot {
  #basePath = new URL(".", location.href).pathname;
  #cache: Promise<Cache> | null = null;
  #customImports: Map<string, string> = new Map();
  #fetchListeners: { test: URLTest; handler: FetchHandler }[] = [];
  #fireListeners: ((sw: ServiceWorker) => void)[] = [];
  #isDev = isLocalhost(location);
  #loaders: Loader[] = [];
  #promises: Promise<any>[] = [];
  #vfs = new VFS();

  get basePath() {
    return this.#basePath;
  }

  get cache() {
    return this.#cache ??
      (this.#cache = caches.open("esm.sh/hot/v" + VERSION));
  }

  get customImports() {
    return this.#customImports;
  }

  /** returns true if the current hostname is localhost */
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

  onLoad(test: RegExp, load: Loader["load"], priority?: string) {
    if (!doc) {
      if (priority === "eager") {
        this.#loaders.unshift({ test, load });
      } else {
        this.#loaders.push({ test, load });
      }
    }
    return this;
  }

  waitUntil(promise: Promise<void>) {
    if (!this.#promises.includes(promise)) {
      this.#promises.push(promise);
    }
  }

  async fire(swName = "/sw.js") {
    if (!doc) {
      throw new Error("Hot.fire() can't be called in Service Worker scope.");
    }

    const sw = navigator.serviceWorker;
    if (!sw) {
      throw new Error("Service Worker not supported.");
    }

    const swUrl = new URL(swName, location.href);
    const reg = await sw.register(swUrl, { type: "module" });

    // update base path to the Service Worker's scope
    this.#basePath = new URL(".", location.href).pathname;

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
            waiting.addEventListener("statechange", () => {
              const { active } = reg;
              if (active) {
                location.reload();
              }
            });
          }
        }
      });
    });

    // detect controller change and refresh the page
    sw.addEventListener("controllerchange", () => {
      location.reload();
    });

    // if there's a waiting, send skip waiting message
    reg.waiting?.postMessage(kSkipWaiting);

    // fire immediately if there's an active Service Worker
    const { active } = reg;
    if (active) {
      this.#fireApp(active);
    }
  }

  async #checkImportMap() {
    const importMap: ImportMap = {
      $support: HTMLScriptElement.supports?.("importmap"),
      imports: Object.fromEntries(this.#customImports.entries()),
    };
    const script = doc.querySelector("head>script[type=importmap]");
    if (script) {
      try {
        const v = JSON.parse(script.innerHTML);
        if (isObject(v)) {
          const { imports, scopes } = v;
          for (const k in imports) {
            importMap.imports![k] = imports[k];
          }
          if (isObject(scopes)) {
            importMap.scopes = scopes;
          }
        }
      } catch (err) {
        console.error("Failed to parse importmap:", err);
      }
    }
    await this.#vfs.put(kImportmapJson, importMap as any);
  }

  async #fireApp(sw: ServiceWorker) {
    const isDev = this.#isDev;
    if (isDev) {
      const { setup } = await import(`./hot-plugins/dev`);
      setup(this);
    }
    await this.#checkImportMap();
    await Promise.all(this.#promises);
    for (const onFire of this.#fireListeners) {
      onFire(sw);
    }
    doc.querySelectorAll("script[type='module/hot']").forEach((el) => {
      const copy = el.cloneNode(true) as HTMLScriptElement;
      copy.type = "module";
      el.replaceWith(copy);
    });
    doc.querySelectorAll("hot-link,hot-script,hot-iframe").forEach((el) => {
      const copy = doc.createElement(el.tagName.slice(4).toLowerCase());
      el.getAttributeNames().forEach((name) => {
        copy.setAttribute(name, el.getAttribute(name)!);
      });
      el.replaceWith(copy);
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
          const load = async () => {
            const res = await fetch(url);
            if (res.ok) {
              const tpl = document.createElement("template");
              tpl.innerHTML = await res.text();
              root.replaceChildren(tpl.content);
            }
          };
          // @ts-ignore
          if (isDev) __hot_hmr_callbacks.set(url.pathname, load);
          load();
        }
      },
    );
    console.log("🔥 app fired.");
  }

  listen() {
    if (doc) {
      throw new Error(
        "Hot.listen() can't be called outside Service Worker scope.",
      );
    }

    const mimeTypes: Record<string, string[]> = {
      "a/gzip": ["gz"],
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
      "t/csv;": ["csv"],
      "t/html;": ["html", "htm"],
      "t/markdown;": ["md", "markdown"],
      "t/plain;": ["txt", "glsl"],
      "t/yaml;": ["yaml", "yml"],
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
        file.meta?.contentType ?? typesMap.get(getExtname(name)) ?? "",
      ]];
      return new Response(file.data, { headers });
    };

    const loaderHeaders = (contentType?: string) => {
      const headers = new Headers();
      headers.set(kContentType, contentType ?? typesMap.get("js")!);
      headers.set(kContentSource, "loader");
      return headers;
    };
    const serveLoader = async (loader: Loader, url: URL, req: Request) => {
      const res = await fetch(req);
      if (!res.ok || res.headers.get(kContentSource) === "loader") {
        return res;
      }
      const resHeaders = res.headers;
      let etag = resHeaders.get("etag");
      if (!etag) {
        const size = resHeaders.get("content-length");
        const modtime = resHeaders.get("last-modified");
        if (size && modtime) {
          etag = "W/" + JSON.stringify(
            parseInt(size).toString(36) + "-" +
              (new Date(modtime).getTime() / 1000).toString(36),
          );
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
        cacheKey = url.pathname.slice(1) + url.search.replaceAll(/=(&|$)/g, "");
      }
      cacheKey = "loader:" + cacheKey;
      let isDev = this.#isDev;
      if (req.headers.get("x-loader-env") === "production") {
        isDev = false;
      }
      if (isDev) {
        cacheKey += (cacheKey.includes("?") ? "&" : "?") + "dev";
      }
      const [record, cached] = await Promise.all([
        vfs.get(kImportmapJson),
        vfs.get(cacheKey),
      ]);
      const importMap: ImportMap = (record?.data as unknown) ?? {};
      const checksum = await computeHash(enc.encode([
        JSON.stringify(importMap),
        etag ?? await source(),
      ].join("")));
      if (cached && cached.meta?.checksum === checksum) {
        if (!res.bodyUsed) {
          res.body?.cancel();
        }
        return new Response(cached.data, {
          headers: loaderHeaders(cached.meta?.contentType),
        });
      }
      try {
        const options = { isDev, importMap };
        const ret = await loader.load(url, await source(), options);
        const { code, map, contentType } = ret;
        let body = code;
        if (map) {
          body +=
            "\n//# sourceMappingURL=data:application/json;charset=utf-8;base64,";
          body += btoa(map);
        }
        vfs.put(cacheKey, body, { checksum, contentType });
        return new Response(body, { headers: loaderHeaders(contentType) });
      } catch (err) {
        console.error(err);
        return new Response(err.message, { status: 500 });
      }
    };

    self.addEventListener("install", (event) => {
      // @ts-ignore
      event.waitUntil(Promise.all(this.#promises));
    });

    self.addEventListener("fetch", (event) => {
      const evt = event as FetchEvent;
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
    });

    self.addEventListener("message", (event) => {
      if (event.data === kSkipWaiting) {
        // @ts-ignore
        self.skipWaiting();
      }
    });
  }
}

function isObject(v: unknown) {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

function isLocalhost({ hostname }: URL | Location) {
  return hostname === "localhost" || hostname === "127.0.0.1";
}

/** get the extension name of the given path */
function getExtname(path: string): string {
  const i = path.lastIndexOf(".");
  if (i >= 0) {
    return path.slice(i + 1);
  }
  return "";
}

/** compute the hash of the given input, default to SHA-1 */
async function computeHash(
  input: Uint8Array,
  algorithm: AlgorithmIdentifier = "SHA-1",
): Promise<string> {
  const buffer = new Uint8Array(await crypto.subtle.digest(algorithm, input));
  return [...buffer].map((b) => b.toString(16).padStart(2, "0")).join("");
}

// 🔥
const hot = new Hot();
export default hot;

// apply plugins
plugins.forEach((plugin) => plugin.setup(hot));

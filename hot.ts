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
  load: (
    url: URL,
    source: string,
    options: { importMap: ImportMap },
  ) => Promise<{
    code: string;
    map?: string;
    headers?: Record<string, string>;
  }>;
  varyUA?: boolean; // for the loaders that checks build target by `user-agent` header
}

interface ImportMap {
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
  hash: string;
  data: string | Uint8Array;
  headers: Record<string, string> | null;
}

const VERSION = 135;
const plugins: Plugin[] = [];
const doc = globalThis.document;
const enc = new TextEncoder();
const kJsxImportSource = "@jsxImportSource";
const kSkipWaiting = "SKIP_WAITING";
const kVfs = "vfs";
const kContentType = "content-type";
const tsQuery = /\?t=[a-z0-9]+$/;

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
    hash: string,
    data: Uint8Array | string,
    headers?: Record<string, string> | null,
  ) {
    const store = await this.#getDbStore("readwrite");
    const req = store.put({
      name,
      hash,
      data,
      headers: headers ?? null,
    });
    return new Promise<void>((resolve, reject) => {
      req.onsuccess = () => resolve();
      req.onerror = () => reject(req.error);
    });
  }

  async #getDbStore(mode: IDBTransactionMode) {
    const db = await this.#dbPromise;
    return db.transaction(kVfs, mode).objectStore(kVfs);
  }
}

/** 🔥 class */
class Hot {
  #vfs = new VFS();
  #cache: Promise<Cache> | null = null;
  #customImports: Map<string, string> = new Map();
  #prefetches = new Set<string>();
  #fetchListeners: { test: URLTest; handler: FetchHandler }[] = [];
  #fireListeners: ((sw: ServiceWorker) => void)[] = [];
  #vfsRegisters: Record<string, (req?: Request) => Promise<VFSRecord>> = {};
  #loaders: Loader[] = [];
  #isDev = location.hostname === "localhost";
  #reloading = false;

  get vfs() {
    return this.#vfs;
  }

  get cache() {
    return this.#cache ??
      (this.#cache = caches.open("esm.sh/hot/v" + VERSION));
  }

  get customImports() {
    return this.#customImports;
  }

  get prefetches() {
    return this.#prefetches;
  }

  /** returns true if the current hostname is localhost */
  get isDev() {
    return this.#isDev;
  }

  /** register a plugin */
  register<T extends string | Uint8Array>(
    name: string,
    load: (req?: Request) =>
      | T
      | Response
      | Promise<T | Response>,
    transform: (input: T) =>
      | T
      | Response
      | Promise<T | Response>,
  ) {
    this.#vfsRegisters[name] = async (req?: Request) => {
      let input = load(req);
      if (input instanceof Promise) {
        input = await input;
      }
      if (input instanceof Response) {
        input = new Uint8Array(await input.arrayBuffer()) as T;
      }
      if (!isString(input) && !(input instanceof Uint8Array)) {
        input = String(input) as T;
      }
      const hash = await computeHash(
        isString(input) ? enc.encode(input) : input,
      );
      const url = this.hotUrl(name);
      const cached = await this.#vfs.get(url);
      if (cached && cached.hash === hash) {
        return cached;
      }
      let data = transform(input);
      if (data instanceof Promise) {
        data = await data;
      }
      if (data instanceof Response) {
        data = new Uint8Array(await data.arrayBuffer()) as T;
      }
      if (cached && doc) {
        if (name.endsWith(".css")) {
          const el = doc.querySelector(`link[href="${url}"]`);
          if (el) {
            const copy = el.cloneNode(true) as HTMLLinkElement;
            copy.href = url + "?" + hash;
            el.replaceWith(copy);
          }
        }
      }
      await this.#vfs.put(url, hash, data);
      return { name, hash, data, headers: null };
    };
    return this;
  }

  onLoad(test: RegExp, load: Loader["load"], varyUA = false) {
    if (!doc) {
      this.#loaders.push({ test, load, varyUA });
    }
    return this;
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

  async fire(swName = "sw.js") {
    if (!doc) {
      throw new Error("Hot.fire() can't be called in Service Worker scope.");
    }

    const sw = navigator.serviceWorker;
    if (!sw) {
      throw new Error("Service Worker not supported.");
    }

    const reg = await sw.register(
      new URL(swName, location.href),
      { type: "module" },
    );

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
                this.#fireApp(active);
              }
            });
          }
        }
      });
    });

    // detect controller change and refresh the page
    sw.addEventListener("controllerchange", () => {
      this.reload();
    });

    // if there's a waiting, send skip waiting message
    reg.waiting?.postMessage(kSkipWaiting);

    // fire immediately if there's an active Service Worker
    const { active } = reg;
    if (active) {
      this.#fireApp(active);
    }
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

    const fetchWithCache = async (req: Request) => {
      if (req.method !== "GET") {
        return fetch(req, { redirect: "manual" });
      }
      const cache = await this.cache;
      let res = await cache.match(req);
      if (res) {
        return res;
      }
      res = await fetch(req, { redirect: "manual" });
      if (!res.ok) {
        return res;
      }
      cache.put(req, res.clone());
      return res;
    };

    const vfs = this.#vfs;
    const serveVFS = async (req: Request, name: string) => {
      const headers: HeadersInit = [[
        kContentType,
        typesMap.get(getExtname(name)) ?? "",
      ]];
      if (name in this.#vfsRegisters) {
        const record = await this.#vfsRegisters[name](req);
        return new Response(record.data, { headers });
      }
      const file = await vfs.get(this.hotUrl(name));
      if (!file) {
        return fetch(req);
      }
      return new Response(file.data, { headers });
    };

    const jsHeaders: HeadersInit = [[kContentType, typesMap.get("js") ?? ""]];
    const noCacheHeaders = { "Cache-Control": "no-cache" };
    const serveLoader = async (loader: Loader, url: URL) => {
      const res = await fetch(url, {
        headers: this.#isDev ? noCacheHeaders : {},
      });
      if (!res.ok) {
        return res;
      }
      const [im, source] = await Promise.all([
        vfs.get("importmap.json"),
        res.text(),
      ]);
      const importMap: ImportMap = (im?.data as unknown) ?? {};
      const jsxImportSource = isJSX(url.pathname)
        ? importMap.imports?.[kJsxImportSource]
        : undefined;
      const cacheKey = this.#isDev && url.host === location.host
        ? url.href.replace(tsQuery, "")
        : url.href;
      const cached = await vfs.get(cacheKey);
      const hash = await computeHash(enc.encode(
        jsxImportSource + source + (loader.varyUA ? navigator.userAgent : ""),
      ));
      if (cached && cached.hash === hash) {
        return new Response(cached.data, {
          headers: cached.headers ?? jsHeaders,
        });
      }
      try {
        const { code, map, headers } = await loader.load(
          url,
          source,
          { importMap },
        );
        let body = code;
        if (map) {
          body +=
            "\n//# sourceMappingURL=data:application/json;charset=utf-8;base64,";
          body += btoa(map);
        }
        vfs.put(cacheKey, hash, body, headers);
        return new Response(body, { headers: headers ?? jsHeaders });
      } catch (err) {
        console.error(err);
        return new Response(err.message, { status: 500 });
      }
    };

    self.addEventListener("install", (event) => {
      // @ts-ignore
      event.waitUntil(
        this.cache.then((cache) => cache.addAll([...this.#prefetches])),
      );
    });

    self.addEventListener("activate", (event) => {
      // @ts-ignore
      event.waitUntil(clients.claim());
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
      if (hostname !== location.hostname) {
        if (hostname === "esm.sh" && pathname.startsWith("/hot/")) {
          evt.respondWith(serveVFS(request, pathname.slice(5)));
        } else {
          evt.respondWith(fetchWithCache(request));
        }
      } else if (
        !url.searchParams.has("raw") && url.pathname !== location.pathname
      ) {
        const loader = loaders.find(({ test }) => test.test(pathname));
        if (loader) {
          evt.respondWith(serveLoader(loader, url));
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

  /** reload the page */
  reload() {
    if (!this.#reloading) {
      this.#reloading = true;
      location.reload();
    }
  }

  hotUrl(name: string) {
    return "https://esm.sh/hot/" + name;
  }

  async #syncVFS() {
    const script = doc.querySelector("head>script[type=importmap]");
    const importMap: ImportMap = {
      imports: Object.fromEntries(this.#customImports.entries()),
    };
    if (script) {
      const supported = HTMLScriptElement.supports?.("importmap");
      const v = JSON.parse(script.innerHTML);
      for (const k in v.imports) {
        if (!supported || k === kJsxImportSource) {
          importMap.imports![k] = v.imports[k];
        }
      }
      if (!supported && "scopes" in v) {
        importMap.scopes = v.scopes;
      }
    }
    await this.#vfs.put(
      "importmap.json",
      await computeHash(enc.encode(JSON.stringify(importMap))),
      importMap as unknown as string,
    );
    await Promise.all(
      Object.values(this.#vfsRegisters).map((handler) => handler()),
    );
  }

  async #fireApp(sw: ServiceWorker) {
    if (this.#isDev) {
      const hmr = await import(`./hot-plugins/hmr`);
      hmr.default.setup(this);
    }
    await this.#syncVFS();
    for (const handler of this.#fireListeners) {
      handler(sw);
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
        constructor() {
          super();
        }
        connectedCallback() {
          const src = this.getAttribute("src");
          if (!src) {
            return;
          }
          const url = new URL(src, location.href);
          const root = this.hasAttribute("shadow")
            ? this.attachShadow({ mode: "open" })
            : this;
          const load = async () => {
            const res = await fetch(url);
            if (res.ok) {
              const tpl = document.createElement("template");
              tpl.innerHTML = await res.text();
              root.replaceChildren(tpl.content);
            }
          };
          // @ts-ignore
          if (hot.hmr) hot.hmrCallbacks.set(url.pathname, load);
          load();
        }
      },
    );
    console.log("🔥 app fired.");
  }
}

/** check if the value is a string */
function isString(v: unknown): v is string {
  return typeof v === "string";
}

/** check if the pathname is a jsx file */
function isJSX(pathname: string): boolean {
  return pathname.endsWith(".jsx") || pathname.endsWith(".tsx");
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
plugins.forEach((plugin) => plugin.setup(hot));
Reflect.set(globalThis, "HOT", hot);
export default hot;

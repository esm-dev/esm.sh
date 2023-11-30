/*! 🔥 esm.sh/hot
 *
 * Documentations: https://esm.sh/hot/docs
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
  load: (url: URL, source: string, options: Record<string, any>) => Promise<{
    code: string;
    map?: string;
    headers?: HeadersInit;
  }>;
  varyUA?: boolean; // for the loaders that checks build target by `user-agent` header
}

interface FetchHandler {
  (req: Request): Response | Promise<Response>;
}

interface UrlTest {
  (url: URL, req: Request): boolean;
}

interface VfsRecord {
  name: string;
  hash: string;
  data: string | Uint8Array;
  headers: [string, string][] | null;
}

interface ImportMap {
  imports?: Record<string, string>;
  scopes?: Record<string, Record<string, string>>;
}

const VERSION = 135;
const plugins: Plugin[] = [];
const doc = globalThis.document;
const enc = new TextEncoder();
const kJsxImportSource = "@jsxImportSource";
const kSkipWaiting = "SKIP_WAITING";
const kVfs = "vfs";

// open indexed database
let onOpen: () => void;
let onOpenError: (reason: DOMException | null) => void;
const openRequest = indexedDB.open("esm.sh/hot", VERSION);
const dbPromise = new Promise<IDBDatabase>((resolve, reject) => {
  onOpen = () => resolve(openRequest.result);
  onOpenError = reject;
});
openRequest.onerror = function () {
  onOpenError(openRequest.error);
};
openRequest.onupgradeneeded = function () {
  const db = openRequest.result;
  if (!db.objectStoreNames.contains(kVfs)) {
    db.createObjectStore(kVfs, { keyPath: "name" });
  }
};
openRequest.onsuccess = function () {
  onOpen();
};

// virtual file system using indexed database
const getVfsStore = async (mode: IDBTransactionMode) => {
  const db = await dbPromise;
  return db.transaction(kVfs, mode).objectStore(kVfs);
};
const vfs = {
  async get(name: string) {
    const store = await getVfsStore("readonly");
    const req = store.get(name);
    return new Promise<VfsRecord | null>(
      (resolve, reject) => {
        req.onsuccess = () => resolve(req.result ? req.result : null);
        req.onerror = () => reject(req.error);
      },
    );
  },
  async put(
    name: string,
    hash: string,
    data: Uint8Array | string,
    headers?: HeadersInit,
  ) {
    const store = await getVfsStore("readwrite");
    const req = store.put({
      name,
      hash,
      data,
      headers: headers ? [...new Headers(headers)] : null,
    });
    return new Promise<void>((resolve, reject) => {
      req.onsuccess = () => resolve();
      req.onerror = () => reject(req.error);
    });
  },
};

// 🔥 class
class Hot {
  loaders: Loader[] = [];
  fetchListeners: { test: UrlTest; handler: FetchHandler }[] = [];
  swListeners: ((sw: ServiceWorker) => void)[] = [];
  vfs: Record<string, (req?: Request) => Promise<VfsRecord>> = {};
  customImports?: Record<string, string>;

  #isDev = location.hostname === "localhost";
  get isDev() {
    return this.#isDev;
  }

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
    this.vfs[name] = async (req?: Request) => {
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
      const url = `https://esm.sh/hot/${name}`;
      const cached = await vfs.get(url);
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
      await vfs.put(url, hash, data);
      return { name, hash, data, headers: null };
    };
    return this;
  }

  onLoad(test: RegExp, load: Loader["load"], varyUA = false) {
    if (!doc) {
      this.loaders.push({ test, load, varyUA });
    }
    return this;
  }

  onFetch(test: UrlTest, handler: FetchHandler) {
    if (!doc) {
      this.fetchListeners.push({ test, handler });
    }
    return this;
  }

  onFire(handler: (reg: ServiceWorker) => void) {
    if (doc) {
      this.swListeners.push(handler);
    }
    return this;
  }

  async fire(swName = "sw.js") {
    if (!doc) {
      throw new Error("Hot.fire() can't be called in Service Worker.");
    }

    const sw = navigator.serviceWorker;
    if (!sw) {
      throw new Error("Service Worker not supported.");
    }

    const reg = await sw.register(new URL(swName, location.href), {
      type: "module",
    });
    const { active, waiting } = reg;

    // detect Service Worker update available and wait for it to become installed
    reg.addEventListener("updatefound", () => {
      reg.installing?.addEventListener("statechange", () => {
        const { waiting } = reg;
        if (waiting) {
          // if there's an existing controller (previous Service Worker)
          if (sw.controller) {
            waiting.postMessage(kSkipWaiting);
          } else {
            // otherwise it's the first install
            // invoke all vfs and store them to the database
            // then reload the page
            this.syncVFS().then(() => {
              reload();
            });
          }
        }
      });
    });

    // detect controller change and refresh the page
    sw.addEventListener("controllerchange", () => {
      reload();
    });

    // there's a waiting, send skip waiting message
    if (waiting) {
      waiting.postMessage(kSkipWaiting);
    }

    // there's an active Service Worker, invoke all listeners
    if (active) {
      for (const handler of this.swListeners) {
        handler(active);
      }
      await this.syncVFS();
      doc.querySelectorAll("script[type='module/hot']").forEach((el) => {
        const copy = el.cloneNode(true) as HTMLScriptElement;
        copy.type = "module";
        el.replaceWith(copy);
      });
      doc.querySelectorAll(
        ["iframe", "script", "link", "style"].map((t) => "hot-" + t).join(","),
      ).forEach(
        (el) => {
          const copy = doc.createElement(el.tagName.slice(4).toLowerCase());
          el.getAttributeNames().forEach((name) => {
            copy.setAttribute(name, el.getAttribute(name)!);
          });
          copy.textContent = el.textContent;
          el.replaceWith(copy);
        },
      );
      console.log("🔥 app fired.");
    }
  }

  async syncVFS() {
    if (doc) {
      const script = doc.querySelector("head>script[type=importmap]");
      const importMap: ImportMap = { imports: { ...this.customImports } };
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
      await vfs.put(
        "importmap.json",
        await computeHash(enc.encode(JSON.stringify(importMap))),
        importMap as unknown as string,
      );
      await Promise.all(
        Object.values(this.vfs).map((handler) => handler()),
      );
    }
  }
}

// 🔥
const hot = new Hot();
plugins.forEach((plugin) => plugin.setup(hot));
Object.assign(globalThis, { HOT: hot });
export default hot;

// service worker environment
if (!doc) {
  const mimeTypes: Record<string, string[]> = {
    "a/gzip": ["gz"],
    "a/javascript": ["js", "mjs"],
    "a/json": ["json", "map"],
    "a/wasm": ["wasm"],
    "a/xml": ["xml"],
    "i/jpeg": ["jpeg", "jpg"],
    "i/png": ["png"],
    "i/svg+xml": ["svg"],
    "t/css": ["css"],
    "t/csv": ["csv"],
    "t/html": ["html", "htm"],
    "t/plain": ["txt", "glsl"],
    "t/yaml": ["yaml", "yml"],
  };
  const alias: Record<string, string> = {
    a: "application",
    i: "image",
    t: "text",
  };
  const typesMap = new Map<string, string>();
  for (const contentType in mimeTypes) {
    for (const ext of mimeTypes[contentType]) {
      typesMap.set(ext, alias[contentType.charAt(0)] + contentType.slice(1));
    }
  }

  let hotCache: Cache | null = null;
  const cacheFetch = async (req: Request) => {
    if (req.method !== "GET") {
      return fetch(req, { redirect: "manual" });
    }
    const cache = hotCache ?? (hotCache = await caches.open("hot/v" + VERSION));
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

  const serveVFS = async (req: Request, name: string) => {
    const headers = { "Content-Type": typesMap.get(getExtname(name)) ?? "" };
    if (name in hot.vfs) {
      const record = await hot.vfs[name](req);
      return new Response(record.data, { headers });
    }
    const file = await vfs.get(`https://esm.sh/hot/${name}`);
    if (!file) {
      return fetch(req);
    }
    return new Response(file.data, { headers });
  };

  const jsHeaders = { "Content-Type": typesMap.get("js") + ";charset=utf-8" };
  const noCacheHeaders = { "Cache-Control": "no-cache" };
  const serveLoader = async (loader: Loader, url: URL) => {
    const res = await fetch(url, { headers: hot.isDev ? noCacheHeaders : {} });
    if (!res.ok) {
      return res;
    }
    const im = await vfs.get("importmap.json");
    const importMap: ImportMap = (im?.data as unknown) ?? {};
    const jsxImportSource = isJsx(url.pathname)
      ? importMap.imports?.[kJsxImportSource]
      : undefined;
    const source = await res.text();
    const cached = await vfs.get(url.href);
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
      await vfs.put(url.href, hash, body, headers);
      return new Response(body, { headers: headers ?? jsHeaders });
    } catch (err) {
      console.error(err);
      return new Response(err.message, { status: 500 });
    }
  };

  self.addEventListener("fetch", (event) => {
    const evt = event as FetchEvent;
    const { request } = evt;
    const url = new URL(request.url);
    const { pathname, hostname } = url;
    const { loaders, fetchListeners } = hot;
    if (fetchListeners.length > 0) {
      for (const { test, handler } of fetchListeners) {
        if (test(url, request)) {
          return evt.respondWith(handler(request));
        }
      }
    }
    if (hostname === "esm.sh") {
      if (pathname.startsWith("/hot/")) {
        evt.respondWith(serveVFS(request, pathname.slice(5)));
      } else {
        evt.respondWith(cacheFetch(request));
      }
    } else if (!url.searchParams.has("raw")) {
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

/** check if the value is a string */
function isString(v: unknown): v is string {
  return typeof v === "string";
}

/** check if the pathname is a jsx file */
function isJsx(pathname: string): boolean {
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

/** reload the page */
let reloading = false;
function reload() {
  if (!reloading) {
    reloading = true;
    location.reload();
  }
}

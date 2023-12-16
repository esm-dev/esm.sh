/*! 🔥 esm.sh/hot
 *  Docs https://esm.sh/hot/docs
 */

/// <reference lib="dom" />
/// <reference lib="webworker" />

import type {
  ContentMap,
  FetchHandler,
  FireOptions,
  HotCore,
  ImportMap,
  Loader,
  Plugin,
  URLTest,
  VFSRecord,
} from "./server/embed/types/hot.d.ts";

const VERSION = 135;
const doc: Document | undefined = globalThis.document;
const loc = location;
const enc = new TextEncoder();
const stringify = JSON.stringify;
const kContentSource = "x-content-source";
const kContentType = "content-type";
const kHot = "esm.sh/hot";
const kHotLoader = "hot-loader";
const kImportmapJson = "internal:importmap.json";
const kSkipWaiting = "SKIP_WAITING";
const kVfs = "vfs";

/** pulgins imported by `?plugins=` query string. */
const plugins: Plugin[] = [];

/** A virtual file system using indexed database. */
class VFS {
  #dbPromise: Promise<IDBDatabase>;

  constructor() {
    const req = indexedDB.open(kHot, VERSION);
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

/** Hot class implements the `HotCore` interface. */
class Hot implements HotCore {
  #basePath = new URL(".", loc.href).pathname;
  #cache: Cache | null = null;
  #importMap: Required<ImportMap> | null = null;
  #contentMap: ContentMap | null = null;
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
    return this.#cache ?? (this.#cache = crateCacheProxy(kHot + VERSION));
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
    priority?: "eager",
  ) {
    if (!doc) {
      this.#loaders[priority ? "unshift" : "push"]({
        test,
        load,
        fetch,
      });
    }
    return this;
  }

  waitUntil(promise: Promise<void>) {
    const promises = this.#promises;
    if (!promises.includes(promise)) {
      promises.push(promise);
    }
  }

  async fire({ plugins, swScript = "/sw.js" }: FireOptions) {
    const sw = navigator.serviceWorker;
    if (!sw) {
      throw new Error("Service Worker not supported.");
    }

    if (this.#fired) {
      console.warn("Got multiple fire() calls, ignored.");
      return;
    }

    // apply plugins
    if (plugins) {
      plugins.forEach((plugin) => plugin.setup(this));
    }

    const isDev = this.#isDev;
    const swScriptUrl = new URL(swScript, loc.href);
    this.#basePath = new URL(".", swScriptUrl).pathname;
    this.#fired = true;

    const reg = await sw.register(swScriptUrl, {
      type: "module",
      updateViaCache: isDev ? undefined : "all",
    });
    const skipWaiting = () => reg.waiting?.postMessage(kSkipWaiting);

    // detect Service Worker update available and wait for it to become installed
    let refreshing = false;
    reg.onupdatefound = () => {
      const { installing } = reg;
      if (installing) {
        installing.onstatechange = () => {
          const { waiting } = reg;
          if (waiting) {
            // if there's an existing controller (previous Service Worker)
            if (sw.controller) {
              // todo: support custom prompt user interface to refresh the page
              skipWaiting();
            } else {
              // otherwise it's the first install
              skipWaiting();
              waiting.onstatechange = () => {
                if (reg.active && !refreshing) {
                  refreshing = true;
                  this.#fireApp(reg.active, true);
                  isDev && console.log("🔥 app registered.");
                }
              };
            }
          }
        };
      }
    };

    // detect controller change and refresh the page
    sw.oncontrollerchange, () => {
      !refreshing && loc.reload();
    };

    // if there's a waiting, send skip waiting message
    skipWaiting();

    // fire immediately if there's an active Service Worker
    if (reg.active) {
      this.#fireApp(reg.active);
    }
  }

  async #fireApp(sw: ServiceWorker, firstActicve = false) {
    const isDev = this.#isDev;
    const promises = this.#promises;

    // load dev plugin if in development mode
    if (isDev) {
      const url = "./hot/dev";
      const { setup } = await import(url);
      setup(this);
    }

    // wait until all promises resolved
    promises.push(this.#vfs.put(kImportmapJson, this.importMap as any));
    await Promise.all(promises);

    // fire all `fire` listeners
    for (const onFire of this.#fireListeners) {
      onFire(sw);
    }

    // reload external css that may be handled by hot-loader
    if (firstActicve) {
      lookupElements<HTMLLinkElement>("link[rel=stylesheet]", (el) => {
        const href = attr(el, "href");
        if (href) {
          const url = new URL(href, loc.href);
          if (isSameOrigin(url)) {
            addTimeStamp(url);
            el.href = url.pathname + url.search;
          }
        }
      });
    }

    lookupElements<HTMLScriptElement>("script", (el) => {
      if (el.type === "text/babel" || el.type === "hot/module") {
        const copy = el.cloneNode(true) as HTMLScriptElement;
        copy.type = "module";
        el.replaceWith(copy);
      }
    });

    defineElement("hot-html", (el) => {
      const src = attr(el, "src");
      if (!src) {
        return;
      }
      const root = el.hasAttribute("shadow")
        ? el.attachShadow({ mode: "open" })
        : el;
      const url = new URL(src, loc.href);
      const load = async (first?: boolean) => {
        if (!first) {
          addTimeStamp(url);
        }
        const res = await fetch(url);
        if (res.ok) {
          const tpl = doc!.createElement("template");
          tpl.innerHTML = await res.text();
          root.replaceChildren(tpl.content);
        } else {
          console.error("Failed to load html from", url);
        }
      };
      if (isDev && isSameOrigin(url)) {
        __hot_hmr_callbacks.add(url.pathname, load);
      }
      load(true);
    });

    defineElement("hot-content", (el) => {
      const is = attr(el, "is");
      if (!is) {
        return;
      }
      const [name, ...path] = is.split(".");
      const contentMap = this.#contentMap ??
        (this.#contentMap = parseContentMap());
      const content = contentMap[name];
      if (!content) {
        return;
      }
      const render = (data: unknown) => {
        const value = lookupValue(
          data,
          path.map((p) =>
            p.split("[").map((expr) => {
              const i = expr.indexOf("]");
              if (i >= 0) {
                const key = expr.slice(0, i);
                if (/^\d+$/.test(key)) {
                  return parseInt(key);
                }
                return key.replace(/^['"]|['"]$/g, "");
              }
              return expr;
            })
          ).flat(),
        );
        el.textContent = !isNullish(value)
          ? value.toString?.() ?? stringify(value)
          : "";
      };
      const { data, cacheTtl } = content;
      if (data && (!data.expires || data.expires > now())) {
        render(data.value);
      } else {
        fetch(this.basePath + "@hot-content", {
          method: "POST",
          body: stringify({ name, ...content }),
        }).then((res) => {
          if (res.ok) {
            res.json().then((value) => {
              content.data = {
                value,
                expires: cacheTtl ? now() + (cacheTtl * 1000) : 0,
              };
              render(value);
            });
          } else {
            console.error("Failed to fetch content", name);
          }
        });
      }
    });

    isDev && console.log("🔥 app fired.");
  }

  listen() {
    // @ts-ignore clients
    if (typeof clients === "undefined") {
      throw new Error("Service Worker scope not found.");
    }

    const mimeTypes: Record<string, string[]> = {
      "a/javascript;": ["js", "mjs"],
      "a/json;": ["json"],
      "a/wasm": ["wasm"],
      "i/gif": ["gif"],
      "i/jpeg": ["jpeg", "jpg"],
      "i/png": ["png"],
      "i/svg+xml;": ["svg"],
      "i/webp": ["webp"],
      "t/css;": ["css"],
      "t/html;": ["html", "htm"],
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
        return createResponse("Not Found", {}, 404);
      }
      const headers: HeadersInit = {
        [kContentType]: file.meta?.contentType ??
          typesMap.get(getExtname(name)) ??
          "binary/octet-stream",
      };
      return createResponse(file.data, headers);
    };
    const loaderHeaders = (contentType?: string) => {
      return new Headers([
        [kContentType, contentType ?? typesMap.get("js")!],
        [kContentSource, kHotLoader],
      ]);
    };
    const serveLoader = async (loader: Loader, url: URL, req: Request) => {
      const res = await (loader.fetch ?? fetch)(req);
      if (!res.ok || res.headers.get(kContentSource) === kHotLoader) {
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
      if (url.host === loc.host) {
        url.searchParams.delete("t");
        cacheKey = url.pathname.slice(1) + url.search.replace(/=(&|$)/g, "");
      }
      let isDev = this.#isDev;
      if (req.headers.get(kHotLoader + "-env") === "production") {
        isDev = false;
      }
      cacheKey = "loader" + (isDev ? "(dev)" : "") + ":" + cacheKey;
      const [vfsImportMap, cached] = await Promise.all([
        vfs.get(kImportmapJson),
        vfs.get(cacheKey),
      ]);
      const importMap: ImportMap = (vfsImportMap?.data as unknown) ?? {};
      const checksum = await computeHash(
        enc.encode(stringify(importMap) + (etag ?? await source())),
      );
      if (cached && cached.meta?.checksum === checksum) {
        if (!res.bodyUsed) {
          res.body?.cancel();
        }
        const headers = loaderHeaders(cached.meta?.contentType);
        headers.set(kHotLoader + "-cache-status", "HIT");
        return createResponse(cached.data, headers);
      }
      try {
        const options = { isDev, importMap };
        const ret = await loader.load(url, await source(), options);
        const { code, contentType, deps, map } = ret;
        let body = code;
        if (map) {
          body += "\n//# sourceMappingURL=data:" + typesMap.get("json") +
            ";base64," + btoa(map);
        }
        vfs.put(cacheKey, body, { checksum, contentType, deps });
        return createResponse(body, loaderHeaders(contentType));
      } catch (err) {
        console.error(err);
        return createResponse(err.message, {}, 500);
      }
    };
    const fetchWithCache = async (req: Request) => {
      const cache = this.cache;
      const cachedReq = await cache.match(req);
      if (cachedReq) {
        return cachedReq;
      }
      const res = await fetch(req.url);
      if (res.status !== 200) {
        return res;
      }
      await cache.put(req, res.clone());
      return res;
    };

    // @ts-ignore disable type check
    self.oninstall = (evt) => evt.waitUntil(Promise.all(this.#promises));

    // @ts-ignore disable type check
    self.onactivate = (evt) => evt.waitUntil(clients.claim());

    // @ts-ignore disable type check
    self.onfetch = (evt: FetchEvent) => {
      const { request } = evt;
      const respondWith = evt.respondWith.bind(evt);
      const url = new URL(request.url);
      const { pathname, hostname } = url;
      const loaders = this.#loaders;
      const fetchListeners = this.#fetchListeners;
      if (fetchListeners.length > 0) {
        for (const { test, handler } of fetchListeners) {
          if (test(url, request)) {
            return respondWith(handler(request));
          }
        }
      }
      if (
        hostname === "esm.sh" &&
        /\w@\d+.\d+\.\d+(-|\/|\?|$)/.test(pathname)
      ) {
        return respondWith(fetchWithCache(request));
      }
      if (hostname === loc.hostname) {
        if (pathname.startsWith("/@hot/")) {
          respondWith(serveVFS(pathname.slice(1)));
        } else if (
          pathname !== loc.pathname &&
          !url.searchParams.has("raw")
        ) {
          const loader = loaders.find(({ test }) => test.test(pathname));
          if (loader) {
            respondWith(serveLoader(loader, url, request));
          }
        }
      }
    };

    self.onmessage = ({ data }) => {
      if (data === kSkipWaiting) {
        // @ts-ignore skipWaiting
        self.skipWaiting();
      }
    };
  }
}

/** get the attribute value of the given element. */
function attr(el: Element, name: string) {
  return el.getAttribute(name);
}

/** look up all elements by the given selectors. */
function lookupElements<T extends Element>(
  selectors: string,
  callback: (value: T) => void,
) {
  // @ts-ignore callback
  doc.querySelectorAll(selectors).forEach(callback);
}

/** define a custom element. */
function defineElement(name: string, callback: (element: HTMLElement) => void) {
  customElements.define(
    name,
    class extends HTMLElement {
      connectedCallback() {
        callback(this);
      }
    },
  );
}

/** query and parse json from <script> with the given type. */
function queryAndParseJSONScript(type: string) {
  const script = doc!.querySelector("head>script[type=" + type + "]");
  if (script) {
    try {
      const v = JSON.parse(script.textContent!);
      if (isObject(v)) {
        return v;
      }
    } catch (err) {
      console.error("Failed to parse JSON of", script, err);
    }
  }
  return null;
}

/** parse importmap from <script> with `type=importmap` */
function parseImportMap() {
  const importMap: Required<ImportMap> = {
    $support: HTMLScriptElement.supports?.("importmap"),
    imports: {},
    scopes: {},
  };
  const obj = queryAndParseJSONScript("importmap");
  if (obj) {
    const { imports, scopes } = obj;
    for (const k in imports) {
      const url = imports[k];
      if (isNEString(url)) {
        importMap.imports[k] = url;
      }
    }
    if (isObject(scopes)) {
      importMap.scopes = scopes;
    }
  }
  return importMap;
}

/** parse contentmap from <script> with `type=contentmap` */
function parseContentMap() {
  const contentMap: ContentMap = {};
  const obj = queryAndParseJSONScript("contentmap");
  if (obj) {
    for (const k in obj) {
      const v = obj[k];
      if (typeof v === "string") {
        contentMap[k] = { url: v };
      } else if (isObject(v) && (isNEString(v.url) || v.data !== undefined)) {
        contentMap[k] = v;
      }
    }
  }
  return contentMap;
}

/** lookup value from the given object by the given path. */
function lookupValue(obj: any, path?: (string | number)[]) {
  let value = obj;
  if (isNullish(value)) {
    return value;
  }
  if (path) {
    for (const key of path) {
      const v = value[key];
      if (v === undefined) {
        return;
      }
      if (typeof v === "function") {
        return v.call(value);
      }
      value = v;
    }
  }
  return value;
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

/** create a response object. */
function createResponse(
  body: BodyInit | null,
  headers: HeadersInit = {},
  status = 200,
): Response {
  return new Response(body, { headers, status });
}

/** check if the given value is nullish. */
function isNullish(v: unknown): v is null | undefined {
  return v === null || v === undefined;
}

/** check if the given value is a non-empty string. */
function isNEString(v: unknown): v is string {
  return typeof v === "string" && v.length > 0;
}

/** check if the given value is an object. */
function isObject(v: unknown): v is Record<string, any> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

/** check if the given url has the same origin with current loc. */
function isSameOrigin(url: URL) {
  return url.origin === loc.origin;
}

/** check if the url is localhost. */
function isLocalhost({ hostname }: URL | Location) {
  return hostname === "localhost" || hostname === "127.0.0.1";
}

/** get current timestamp. */
function now() {
  return Date.now();
}

/** add timestamp to the given url. */
function addTimeStamp(url: URL) {
  url.searchParams.set("t", now().toString(36));
}

/** get the extension name of the given path. */
function getExtname(path: string): string {
  const i = path.lastIndexOf(".");
  return i >= 0 ? path.slice(i + 1) : "";
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

export default new Hot(plugins);

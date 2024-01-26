/*! 🔥 esm.sh/hot
 *  Docs https://docs.esm.sh/hot
 */

/// <reference lib="dom" />
/// <reference lib="dom.iterable" />
/// <reference lib="webworker" />

import type {
  FetchHandler,
  HotCore,
  HotMessageChannel,
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
const parse = JSON.parse;
const stringify = JSON.stringify;
const kContentSource = "x-content-source";
const kContentType = "content-type";
const kHot = "esm.sh/hot";
const kHotLoader = "hot-loader";
const kSkipWaiting = "SKIP_WAITING";
const kMessage = "message";
const kVfs = "vfs";

/** pulgins imported by `?plugins=` query. */
const plugins: Plugin[] = [];

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
    return waitIDBRequest<VFSRecord | undefined>(tx.get(name));
  }

  async put(name: string, data: VFSRecord["data"], meta?: VFSRecord["meta"]) {
    const record: VFSRecord = { name, data };
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
  #basePath = new URL(".", loc.href).pathname;
  #cache: Cache | null = null;
  #importMap: Required<ImportMap> | null = null;
  #fetchListeners: { test: URLTest; handler: FetchHandler }[] = [];
  #fireListeners: ((sw: ServiceWorker) => void)[] = [];
  #isDev = isLocalhost(location);
  #loaders: Loader[] = [];
  #promises: Promise<any>[] = [];
  #vfs = new VFS(kHot, VERSION);
  #contentCache: Record<string, any> = {};
  #fired = false;
  #activatedSW: ServiceWorker | null = null;

  constructor(plugins: Plugin[] = []) {
    plugins.forEach((plugin) => plugin.setup(this));
  }

  get basePath() {
    return this.#basePath;
  }

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

  onFetch(test: URLTest, handler: FetchHandler) {
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

  onLoad(
    test: RegExp,
    load: Loader["load"],
    fetch?: Loader["fetch"],
    priority?: "eager",
  ) {
    if (!doc) {
      this.#loaders[priority ? "unshift" : "push"]({ test, load, fetch });
    }
    return this;
  }

  openMessageChannel(channelName: string): Promise<HotMessageChannel> {
    const url = this.basePath + "@hot-events?channel=" + channelName;
    const conn = new EventSource(url);
    return new Promise((resolve, reject) => {
      const mc: HotMessageChannel = {
        onMessage: (handler) => {
          const msgHandler = (evt: MessageEvent) => {
            handler(parse(evt.data));
          };
          conn.addEventListener(kMessage, msgHandler);
          return () => {
            conn.removeEventListener(kMessage, msgHandler);
          };
        },
        postMessage: (data) => {
          return fetch(url, {
            method: "POST",
            body: stringify(data ?? null),
          }).then((res) => res.ok);
        },
        close: () => {
          conn.close();
        },
      };
      conn.onopen = () => resolve(mc);
      conn.onerror = () =>
        reject(
          new Error(`Failed to open message channel "${channelName}"`),
        );
    });
  }

  waitUntil(promise: Promise<void>) {
    this.#promises.push(promise);
  }

  async fire(swScript = "/sw.js") {
    const sw = navigator.serviceWorker;
    if (!sw) {
      throw new Error("Service Worker not supported.");
    }

    if (this.#fired) {
      return;
    }

    const isDev = this.#isDev;
    const swScriptUrl = new URL(swScript, loc.href);
    this.#basePath = new URL(".", swScriptUrl).pathname;
    this.#fired = true;

    const v = this.importMap.scopes?.[swScript]?.["@hot"];
    if (v) {
      swScriptUrl.searchParams.set("@hot", v);
    }
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

    // load dev plugin if in development mode
    if (isDev) {
      const url = "./hot/dev";
      const { setup } = await import(url);
      setup(this);
    }

    // wait until all promises resolved
    sw.postMessage(this.importMap);
    await Promise.all(this.#promises);

    // fire all `fire` listeners
    for (const handler of this.#fireListeners) {
      handler(sw);
    }
    this.#activatedSW = sw;

    // reload external css that may be handled by hot-loader
    if (firstActicve) {
      queryElements<HTMLLinkElement>("link[rel=stylesheet]", (el) => {
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

    // apply "text/babel" and "hot/module" script tags
    queryElements<HTMLScriptElement>("script", (el) => {
      if (el.type === "text/babel" || el.type === "hot/module") {
        const copy = el.cloneNode(true) as HTMLScriptElement;
        copy.type = "module";
        el.replaceWith(copy);
      }
    });

    // <use-html src="./pages/foo.html" ssr></use-html>
    // <use-html src="./blog/foo.md" ssr></use-html>
    // <use-html src="./icons/foo.svg" ssr></use-html>
    defineElement("use-html", (el) => {
      const src = attr(el, "src");
      if (!src) {
        return;
      }
      const root = el.hasAttribute("shadow")
        ? el.attachShadow({ mode: "open" })
        : el;
      const url = new URL(src, loc.href);
      const { pathname, searchParams } = url;
      if ([".md", ".markdown"].some((ext) => pathname.endsWith(ext))) {
        searchParams.set("html", "");
      }
      const load = async (hmr?: boolean) => {
        if (hmr) {
          addTimeStamp(url);
        }
        const res = await fetch(url);
        const text = await res.text();
        root.innerHTML = res.ok ? text : createErrorTag(text);
      };
      if (isDev && isSameOrigin(url)) {
        __hot_hmr_callbacks.add(pathname, () => load(true));
      }
      load();
    });

    // <use-content src="foo" map="this.bar" ssr></use-content>
    defineElement("use-content", (el) => {
      const src = attr(el, "src");
      if (!src) {
        return;
      }
      if (el.hasAttribute("store")) {
        return;
      }
      const cache = this.#contentCache;
      const render = (data: unknown) => {
        if (data instanceof Error) {
          el.innerHTML = createErrorTag(data[kMessage]);
          return;
        }
        const mapKey = attr(el, "mapKey");
        const content = toString(
          mapKey && !isNullish(data) ? (data as any)[mapKey] : data,
        );
        if (el.hasAttribute("html")) {
          el.innerHTML = content;
        } else {
          el.textContent = content;
        }
      };
      const load = () => {
        const renderedData = cache[src];
        if (renderedData) {
          if (renderedData instanceof Promise) {
            renderedData.then(render);
          } else {
            render(renderedData);
          }
        } else {
          cache[src] = fetch(this.basePath + "@hot-content", {
            method: "POST",
            body: stringify({ src, location: location.pathname }),
          }).then(async (res) => {
            if (res.ok) {
              const value = await res.json();
              cache[src] = value;
              return render(value);
            }
            let msg = res.statusText;
            const text = (await res.text()).trim();
            if (text) {
              msg = text;
              if (text.trimStart().startsWith("{")) {
                try {
                  const { error, message } = parse(text);
                  msg = error?.[kMessage] ?? message ?? msg;
                } catch (_) {
                  // ignore
                }
              }
            }
            delete cache[src];
            render(new Error(msg));
          });
        }
      };
      const liveProp = attr(el, "live");
      if (liveProp) {
        const live = parseInt(liveProp);
        if (live > 0) {
          const check = () => {
            delete cache[src];
            load();
          };
          setInterval(check, 1000 * live);
        }
      }
      load();
    });

    isDev && console.log("🔥 app fired.");
  }

  use(...plugins: Plugin[]) {
    plugins.forEach((plugin) => plugin.setup(this));
    return this;
  }

  listen() {
    // @ts-ignore clients
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
        [kContentType]: file.meta?.contentType ?? "binary/octet-stream",
      };
      return createResponse(file.data, headers);
    };
    const loaderHeaders = (contentType?: string) => {
      return new Headers([
        [kContentType, contentType ?? "application/javascript; charset=utf-8"],
        [kContentSource, kHotLoader],
      ]);
    };
    const serveLoader = async (loader: Loader, url: URL, req: Request) => {
      const res = await (loader.fetch ?? fetch)(req);
      if (!res.ok || res.headers.get(kContentSource) === kHotLoader) {
        return res;
      }
      const resHeaders = res.headers;
      const etag = resHeaders.get("etag");
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
      const importMap = this.importMap;
      const cached = await vfs.get(cacheKey);
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
          body += "\n//# sourceMappingURL=data:application/json" +
            ";base64," + btoa(map);
        }
        vfs.put(cacheKey, body, { checksum, contentType, deps });
        return createResponse(body, loaderHeaders(contentType));
      } catch (err) {
        return createResponse(err[kMessage], {}, 500);
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

    // @ts-ignore listen to SW `install` event
    self.oninstall = (evt) => evt.waitUntil(Promise.all(this.#promises));

    // @ts-ignore listen to SW `activate` event
    self.onactivate = (evt) => evt.waitUntil(clients.claim());

    // @ts-ignore listen to SW `fetch` event
    self.onfetch = (evt: FetchEvent) => {
      const { request } = evt;
      const respondWith = evt.respondWith.bind(evt);
      const url = new URL(request.url);
      const { pathname } = url;
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
        url.hostname === "esm.sh" && /\w@\d+.\d+\.\d+(-|\/|\?|$)/.test(pathname)
      ) {
        return respondWith(fetchWithCache(request));
      }
      if (isSameOrigin(url)) {
        if (pathname.startsWith("/@hot/")) {
          respondWith(serveVFS(pathname.slice(1)));
        } else if (pathname !== loc.pathname && !url.searchParams.has("raw")) {
          const loader = loaders.find(({ test }) => test.test(pathname));
          if (loader) {
            respondWith(serveLoader(loader, url, request));
          }
        }
      }
    };

    // listen to SW `message` event for `skipWaiting` control on renderer process
    self.onmessage = ({ data }) => {
      if (data === kSkipWaiting) {
        // @ts-ignore skipWaiting
        self.skipWaiting();
      } else if (isObject(data) && data.imports) {
        this.#importMap = data as Required<ImportMap>;
      }
    };
  }
}

/** get the attribute value of the given element. */
function attr(el: Element, name: string) {
  return el.getAttribute(name);
}

/** query all elements by the given selectors. */
function queryElements<T extends Element>(
  selectors: string,
  callback: (value: T) => void,
) {
  // @ts-ignore callback
  doc.querySelectorAll(selectors).forEach(callback);
}

/** define a custom element. */
function defineElement(
  name: string,
  callback?: (element: HTMLElement) => void,
) {
  customElements.define(
    name,
    class extends HTMLElement {
      connectedCallback() {
        callback?.(this);
      }
    },
  );
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
  let json = null;
  if (script) {
    try {
      json = parse(script.textContent!);
    } catch (err) {
      console.error("Invalid importmap", err[kMessage]);
    }
  }
  if (isObject(json)) {
    const { imports, scopes } = json;
    for (const k in imports) {
      const url = imports[k];
      if (url) {
        importMap.imports[k] = url;
      }
    }
    if (isObject(scopes)) {
      importMap.scopes = scopes;
    }
  }
  return importMap;
}

/** create a error tag. */
function createErrorTag(msg: string) {
  return `<code style="color:red">${msg}</code>`;
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

/** check if the given value is nullish. */
function isNullish(v: unknown): v is null | undefined {
  return v === null || v === undefined;
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

/** convert the given value to string. */
function toString(value: unknown) {
  if (isNullish(value)) {
    return "";
  }
  if (typeof value === "string") {
    return value;
  }
  return (value as any).toString?.() ?? stringify(value);
}

/** add timestamp to the given url. */
function addTimeStamp(url: URL) {
  url.searchParams.set("t", Date.now().toString(36));
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
) {
  const buffer = new Uint8Array(await crypto.subtle.digest(algorithm, input));
  return [...buffer].map((b) => b.toString(16).padStart(2, "0")).join("");
}

export default new Hot(plugins);

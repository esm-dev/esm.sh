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
const obj = Object;
const parse = JSON.parse;
const stringify = JSON.stringify;
const kContentSource = "x-content-source";
const kContentType = "content-type";
const kHot = "esm.sh/hot";
const kHotLoader = "hot-loader";
const kSkipWaiting = "SKIP_WAITING";
const kMessage = "message";
const kVfs = "vfs";
const symWatch = Symbol();
const symAnyKey = Symbol();

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
  #firedSW: ServiceWorker | null = null;
  #state: Record<string, unknown> | null = null;

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

  state(
    init: Record<string, unknown> | Promise<Record<string, unknown>>,
  ): void {
    if (init instanceof Promise) {
      this.#promises.push(init.then((state) => this.state(state)));
    } else if (isObject(init)) {
      this.#state = createStore(init);
    }
  }

  onFetch(test: URLTest, handler: FetchHandler) {
    if (!doc) {
      this.#fetchListeners.push({ test, handler });
    }
    return this;
  }

  onFire(handler: (reg: ServiceWorker) => void) {
    if (doc) {
      if (this.#firedSW) {
        handler(this.#firedSW);
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
    this.#firedSW = sw;

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
      if (attr(el, "_ssr") === "1") {
        return;
      }
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
        if (res.ok) {
          setInnerHtml(root, text);
        } else {
          setInnerHtml(root, createErrorTag(text));
        }
      };
      if (isDev && isSameOrigin(url)) {
        __hot_hmr_callbacks.add(pathname, () => load(true));
      }
      load();
    });

    // <use-content src="foo" map="this.bar" ssr></use-content>
    defineElement("use-content", (el) => {
      if (attr(el, "_ssr") === "1") {
        return;
      }
      const src = attr(el, "src");
      if (!src) {
        return;
      }
      const render = (data: unknown) => {
        if (data instanceof Error) {
          setInnerHtml(el, createErrorTag(data[kMessage]));
          return;
        }
        const mapExpr = attr(el, "map");
        const value = mapExpr && !isNullish(data)
          ? new Function("return " + mapExpr).call(data)
          : data;
        setInnerHtml(el, toString(value));
      };
      const cache = this.#contentCache;
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
            render(value);
            return;
          }
          let msg = res.statusText;
          try {
            const text = (await res.text()).trim();
            if (text) {
              msg = text;
              if (isBlockString(text.trim())) {
                const { error, message } = parse(text);
                msg = error?.[kMessage] ?? message ?? msg;
              }
            }
          } catch (_) {
            // ignore
          }
          delete cache[src];
          render(new Error(msg));
        });
      }
    });

    // <use-state init="{foo: "bar"}">{foo}</use-state>
    defineElement("use-state", (root) => {
      const globalState = this.#state;
      const inhertScopes = get(root, "$scopes") ??
        (globalState ? [globalState] : []);
      const init = new Function(
        "$scope",
        "return " + (attr(root, "onload") ?? "null"),
      )(
        new Proxy(Object.create(null), {
          get: (_, key) => findOwn(inhertScopes, key)?.[key],
          set: (_, key, value) => {
            const s = findOwn(inhertScopes, key) ?? inhertScopes[0];
            return s ? set(s, key, value) : false;
          },
        }),
      );
      const scopes = [
        ...(isObject(init) ? [createStore(init)] : []),
        ...inhertScopes,
      ];
      const interpret = (
        $scopes: Record<string, unknown>[],
        expr: string,
        update: (content: string) => void,
        blockStart?: string,
        blockEnd?: string,
      ) => {
        const [texts, blocks] = splitByBlocks(
          expr,
          blockStart,
          blockEnd,
        );
        if (blocks.length > 0) {
          const effect = (watch?: boolean) => {
            update(
              texts.map((text, i) => {
                const block = blocks[i];
                if (block) {
                  const m = block.match(
                    /^([\w$]+)(\s*[\[\^+\-*/%<>|&.?].+)?$/,
                  );
                  if (m) {
                    const [, ident, accesser] = m;
                    const scope = findOwn($scopes, ident);
                    if (scope) {
                      if (watch) {
                        scope[symWatch]?.(ident, effect);
                      }
                      let value = get(scope, ident);
                      if (accesser) {
                        value = new Function(ident, "return " + block)(value);
                        if (value === false) {
                          value = "";
                        }
                      }
                      return text + toString(value);
                    }
                  }
                }
                return text;
              }).join(""),
            );
          };
          effect(true);
        }
      };
      const reactive = (el: Element, currentScope?: any) => {
        let $scopes = scopes;
        if (currentScope) {
          $scopes = [currentScope, ...scopes];
        }
        const activeNode = (node: ChildNode) => {
          if (node.nodeType === 1 /* element node */) {
            const el = node as Element;
            const tagName = el.tagName.toLowerCase();
            const props = attrs(el);

            // nested <use-state> tag
            if (tagName === "use-state") {
              Object.assign(el, { $scopes });
              return false;
            }

            const boolAttrs = new Set<string>();
            const commonAttrs = new Set<string>();
            const eventAttrs = new Set<string>();

            for (const prop of props) {
              if (attr(el, prop) === "") {
                boolAttrs.add(prop);
              } else if (prop.startsWith("on")) {
                eventAttrs.add(prop);
              } else {
                commonAttrs.add(prop);
              }
            }

            // list rendering
            if (commonAttrs.has("for")) {
              const [iter, key] = attr(el, "for")!.split(" of ").map((s) =>
                s.trim()
              );
              if (iter && key) {
                const templateEl = el;
                const keyProp = attr(templateEl, "key");
                const placeholder = doc!.createComment("&")!;
                const scope = findOwn($scopes, key) ?? $scopes[0];
                let marker: Element[] = [];
                const renderList = () => {
                  const arr = get(scope, key);
                  if (Array.isArray(arr)) {
                    let iterKey = iter;
                    let iterIndex = "";
                    if (isBlockString(iterKey, "(", ")")) {
                      [iterKey, iterIndex] = iterKey.slice(1, -1).split(",", 2)
                        .map((s) => s.trim());
                    }
                    const render = () => {
                      const map = new Map<string, Element>();
                      for (const el of marker) {
                        const key = attr(el, "key");
                        key && map.set(key, el);
                      }
                      const listEls = arr.map((item, index) => {
                        const iterScope: Record<string, unknown> = {};
                        if (iterKey) {
                          iterScope[iterKey] = item;
                        }
                        if (iterIndex) {
                          iterScope[iterIndex] = index;
                        }
                        if (keyProp && map.size > 0) {
                          let key = "";
                          interpret(
                            [iterScope, ...$scopes],
                            keyProp,
                            (ret) => {
                              key = ret;
                            },
                          );
                          const sameKeyEl = map.get(key);
                          if (sameKeyEl) {
                            return sameKeyEl;
                          }
                        }
                        const listEl = templateEl.cloneNode(true) as Element;
                        reactive(listEl, iterScope);
                        return listEl;
                      });
                      marker.forEach((el) => el.remove());
                      listEls.forEach((el) => placeholder.before(el));
                      marker = listEls;
                    };
                    render();
                    get(arr, symWatch)(symAnyKey, render);
                  } else if (marker.length > 0) {
                    marker.forEach((el) => el.remove());
                    marker.length = 0;
                  }
                };
                el.replaceWith(placeholder);
                templateEl.removeAttribute("for");
                renderList();
                scope[symWatch](key, renderList);
              }
              return false;
            }

            // render properties with state
            for (const prop of commonAttrs) {
              const isStyle = prop === "style";
              interpret(
                $scopes,
                attr(el, prop) ?? "",
                (content) => el.setAttribute(prop, content),
                isStyle ? "state(" : undefined,
                isStyle ? ")" : undefined,
              );
            }

            // conditional rendering
            let cProp = "";
            let notOp = false;
            for (let prop of boolAttrs) {
              notOp = prop.startsWith("!");
              if (notOp) {
                prop = prop.slice(1);
              }
              if (findOwn($scopes, prop)) {
                cProp = prop;
                break;
              }
            }
            if (cProp) {
              const scope = findOwn($scopes, cProp) ?? $scopes[0];
              const cEl = el;
              const placeholder = doc!.createComment("&")!;
              let anchor: ChildNode = el;
              const switchEl = (nextEl: ChildNode) => {
                if (nextEl !== anchor) {
                  anchor.replaceWith(nextEl);
                  anchor = nextEl;
                }
              };
              const toggle = () => {
                let ok = get(scope!, cProp!);
                if (notOp) {
                  ok = !ok;
                }
                if (ok) {
                  switchEl(cEl);
                } else {
                  switchEl(placeholder);
                }
              };
              toggle();
              scope[symWatch](cProp, toggle);
            }

            // bind scopes for event handlers
            if (eventAttrs.size > 0) {
              const marker = new Set<string>();
              for (const scope of $scopes) {
                const keys = obj.keys(scope);
                for (const key of keys) {
                  if (!marker.has(key)) {
                    marker.add(key);
                    !Object.hasOwn(el, key) &&
                      Object.defineProperty(el, key, {
                        get: () => get(scope, key),
                        set: (value) => set(scope, key, value),
                      });
                  }
                }
              }
              // apply event modifiers if exists
              for (const a of eventAttrs) {
                let handler = attr(el, a);
                if (handler) {
                  const [event, ...rest] = a.toLowerCase().split(".");
                  const modifiers = new Set(rest);
                  const addCode = (code: string) => {
                    handler = code + handler;
                  };
                  if (modifiers.size) {
                    if (modifiers.delete("once")) {
                      addCode(`this.removeAttribute('${event}');`);
                    }
                    if (modifiers.delete("prevent")) {
                      addCode("event.preventDefault();");
                    }
                    if (modifiers.delete("stop")) {
                      addCode("event.stopPropagation();");
                    }
                    if (event.startsWith("onkey") && modifiers.size) {
                      if (modifiers.delete("space")) {
                        modifiers.add(" ");
                      }
                      addCode(
                        "if (!['" +
                          ([...modifiers.values()].join("','")) +
                          "'].includes(event.code.toLowerCase()))return;",
                      );
                    }
                    el.removeAttribute(a);
                    el.setAttribute(event, handler);
                  }
                }
              }
            }

            // bind state for input, select and textarea elements
            if (
              tagName === "input" ||
              tagName === "select" ||
              tagName === "textarea"
            ) {
              const inputEl = el as
                | HTMLInputElement
                | HTMLSelectElement
                | HTMLTextAreaElement;
              const name = attr(inputEl, "name");
              if (name) {
                const scope = findOwn($scopes, name);
                if (scope) {
                  const type = attr(inputEl, "type");
                  const isCheckBox = type === "checkbox";
                  const update = () => {
                    const value = get(scope, name);
                    if (isCheckBox) {
                      (inputEl as HTMLInputElement).checked = !!value;
                    } else {
                      inputEl.value = toString(value);
                    }
                  };
                  let convert: (input: string) => unknown = String;
                  if (type === "number") {
                    convert = Number;
                  } else if (isCheckBox) {
                    convert = (_input) => (inputEl as HTMLInputElement).checked;
                  }
                  update();
                  const dispose = scope[symWatch](name, update, true);
                  inputEl.addEventListener("input", () => {
                    const recover = dispose();
                    set(scope, name, convert(inputEl.value));
                    recover();
                  });
                }
              }
            }
          } else if (node.nodeType === 3 /* text node */) {
            interpret(
              $scopes,
              node.textContent ?? "",
              (content) => node.textContent = content,
            );
          }
        };
        if (el !== root) {
          activeNode(el);
        }
        walkNodes(el, activeNode);
      };
      reactive(root);
    });

    doc!.head.appendChild(doc!.createElement("style")).append(
      "use-state{visibility: visible;}",
    );

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

function createStore<T extends object>(init: T) {
  const watchers: Record<string | symbol, Set<() => void>> = {};
  const watch = (
    key: string | symbol,
    handler: () => void,
    disposable?: boolean,
  ) => {
    const set = watchers[key] ?? (watchers[key] = new Set());
    const add = () => set.add(handler);
    add();
    if (disposable) {
      return () => { // dispose
        set.delete(handler);
        return add; // recover
      };
    }
  };
  let filled = false;
  let flushPending = false;
  const flushKeys: Set<string | symbol> = new Set();
  const flush = () => {
    [...flushKeys, symAnyKey].forEach((key) => {
      watchers[key]?.forEach((handler) => handler());
    });
    flushKeys.clear();
    flushPending = false;
  };
  const store = new Proxy(Array.isArray(init) ? [] : Object.create(null), {
    get: (target, key) => {
      if (key === symWatch) {
        return watch;
      }
      return get(target, key);
    },
    set: (target, key, value) => {
      if (typeof value === "object" && value !== null) {
        value = createStore(value);
      }
      const ok = set(target, key, value);
      if (ok && filled) {
        flushKeys.add(key);
        if (!flushPending) {
          flushPending = true;
          queueMicrotask(flush);
        }
      }
      return ok;
    },
  });
  for (const [key, value] of Object.entries(init)) {
    store[key] = value;
  }
  filled = true;
  return store as T;
}

function findOwn(list: any[], key: PropertyKey) {
  return list.find((o) => Reflect.has(o, key));
}

/** get the attribute value of the given element. */
function attr(el: Element, name: string) {
  return el.getAttribute(name);
}

/** get all attribute names of the given element. */
function attrs(el: Element) {
  return el.getAttributeNames();
}

/** query all elements by the given selectors. */
function queryElements<T extends Element>(
  selectors: string,
  callback: (value: T) => void,
) {
  // @ts-ignore callback
  doc.querySelectorAll(selectors).forEach(callback);
}

/** walk all nodes recursively. */
function walkNodes(
  { childNodes }: Element,
  handler: (node: ChildNode) => void | false,
) {
  childNodes.forEach((node) => {
    if (handler(node) !== false && node.nodeType === 1) {
      walkNodes(node as Element, handler);
    }
  });
}

/** split the given expression by blocks. */
function splitByBlocks(expr: string, bloackStart = "{", blockEnd = "}") {
  const texts: string[] = [];
  const blocks: string[] = [];
  let i = 0;
  let j = 0;
  while (i < expr.length) {
    j = expr.indexOf(bloackStart, i);
    if (j === -1) {
      texts.push(expr.slice(i));
      break;
    }
    texts.push(expr.slice(i, j));
    i = expr.indexOf(blockEnd, j);
    if (i === -1) {
      texts[texts.length - 1] += expr.slice(j);
      break;
    }
    const ident = expr.slice(j + bloackStart.length, i).trim();
    if (ident) {
      blocks.push(ident);
    }
    i++;
  }
  return [texts, blocks];
}

/** check if the given text is a block string. */
function isBlockString(text: string, bloackStart = "{", blockEnd = "}") {
  return text.startsWith(bloackStart) && text.endsWith(blockEnd);
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

/** set innerHTML of the given element. */
function setInnerHtml(el: HTMLElement | ShadowRoot, html: string) {
  el.innerHTML = html;
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

/** get the given property of the given target. */
function get(target: object, key: PropertyKey) {
  return Reflect.get(target, key);
}

/** set the given property of the given target. */
function set(target: object, key: PropertyKey, value: unknown) {
  return Reflect.set(target, key, value);
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

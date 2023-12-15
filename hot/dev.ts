import type { CallbackMap, Hot } from "../server/embed/types/hot.d.ts";

class CallbackMapImpl<T extends Function> implements CallbackMap<T> {
  map = new Map<string, Set<T>>();
  add(path: string, callback: T) {
    const map = this.map;
    (map.get(path) ?? map.set(path, new Set()).get(path)!).add(callback);
  }
  delete(path: string, callback?: T) {
    const map = this.map;
    if (callback) {
      if (map.get(path)?.delete(callback)) {
        if (map.get(path)!.size === 0) {
          map.delete(path);
        }
      }
    } else {
      map.delete(path);
    }
  }
}

export function setup(hot: Hot) {
  globalThis.__hot_hmr_modules = new Set();
  globalThis.__hot_hmr_callbacks = new CallbackMapImpl();
  globalThis.__hot_hmr_disposes = new CallbackMapImpl();

  hot.importMap.imports["@hmrRuntime"] = "/@hot/hmr.js";
  hot.waitUntil(hot.vfs.put(
    "@hot/hmr.js",
    `
    const registry = new Map();

    class Context {
      constructor(path) {
        this.path = path;
        this.locked = false;
      }
      lock() {
        this.locked = true;
      }
      accept(cb) {
        if (this.locked) {
          return;
        }
        __hot_hmr_modules.add(this.path);
        typeof cb === "function" && __hot_hmr_callbacks.add(this.path, cb);
      }
      dispose(cb) {
        typeof cb === "function" && __hot_hmr_disposes.add(this.path, cb);
      }
      invalidate() {
        location.reload();
      }
    }

    export default (path) => {
      let ctx = registry.get(path);
      if (ctx) {
        ctx.lock();
        return ctx;
      }
      ctx = new Context(path);
      registry.set(path, ctx);
      return ctx;
    };
    `,
  ));

  hot.importMap.imports["@reactRefreshRuntime"] = "/@hot/hmr_react_refresh.js";
  hot.waitUntil(hot.vfs.put(
    "@hot/hmr_react_refresh.js",
    `
    // react-refresh
    // @link https://github.com/facebook/react/issues/16604#issuecomment-528663101

    import runtime from "https://esm.sh/v135/react-refresh@0.14.0/runtime";

    let timer;
    const refresh = () => {
      if (timer !== null) {
        clearTimeout(timer);
      }
      timer = setTimeout(() => {
        runtime.performReactRefresh();
        timer = null;
      }, 30);
    };

    runtime.injectIntoGlobalHook(window);
    window.$RefreshReg$ = () => {};
    window.$RefreshSig$ = () => type => type;

    export { refresh as __REACT_REFRESH__, runtime as __REACT_REFRESH_RUNTIME__ };
    `,
  ));

  hot.onFire(() => {
    const logPrefix = ["🔥 %c[HMR]", "color:#999"];
    const eventColors = {
      modify: "#056CF0",
      create: "#20B44B",
      remove: "#F00C08",
    };

    let connected = false;
    const es = new EventSource(
      new URL(hot.basePath + "@hot-notify", location.href),
    );

    es.addEventListener("fs-notify", async (evt) => {
      const { type, name } = JSON.parse(evt.data);
      const accepted = __hot_hmr_modules.has(name);
      const callbacks = __hot_hmr_callbacks.map.get(name);
      if (type === "modify") {
        if (accepted) {
          const disposes = __hot_hmr_disposes.map.get(name);
          if (disposes) {
            disposes.clear();
            disposes.forEach((cb) => cb());
          }
          const url = new URL(name, location.href);
          url.searchParams.set("t", Date.now().toString(36));
          if (url.pathname.endsWith(".css")) {
            url.searchParams.set("module", "");
          }
          const module = await import(url.href);
          if (callbacks) {
            callbacks.forEach((cb) => cb(module));
          }
        } else if (callbacks) {
          callbacks.forEach((cb) => cb(null));
        }
      }
      if (accepted || callbacks) {
        console.log(
          logPrefix[0] + " %c" + type,
          logPrefix[1],
          `color:${eventColors[type as keyof typeof eventColors]}`,
          `${JSON.stringify(name)}`,
        );
      }
    });

    es.onopen = () => {
      if (!connected) {
        import(new URL("./devtools", import.meta.url).href)
          .then(({ render }) => render(hot));
      }
      connected = true;
      console.log(
        ...logPrefix,
        "connected, listening for file changes...",
      );
    };

    es.onerror = (err) => {
      if (err.eventPhase === EventSource.CLOSED) {
        if (!connected) {
          console.warn(...logPrefix, "failed to connect.");
        } else {
          console.log(...logPrefix, "connection lost, reconnecting...");
        }
      }
    };

    // enable css hmr
    document.querySelectorAll("link[rel=stylesheet]").forEach((el) => {
      let link = el as HTMLLinkElement;
      const url = new URL(link.href, location.href);
      if (url.hostname === location.hostname) {
        const reload = () => {
          const next = new URL(url);
          next.searchParams.set("t", Date.now().toString(36));
          const oldLink = link;
          const newLink = oldLink.cloneNode() as HTMLLinkElement;
          newLink.href = next.href;
          newLink.onload = () => {
            setTimeout(() => {
              oldLink.remove();
            }, 0);
            watchDeps();
          };
          oldLink.parentNode?.insertBefore(newLink, oldLink.nextSibling);
          link = newLink;
        };
        const vfsKey = `loader(dev):${url.pathname.slice(1)}`;
        const watchDeps = async () => {
          const disposes: (() => void)[] = [];
          const res = await hot.vfs.get(vfsKey);
          const { deps } = res?.meta ?? {};
          deps?.forEach((dep) => {
            const specifier = typeof dep === "string" ? dep : dep.specifier;
            if (specifier.startsWith("/")) {
              const onChange = () => {
                disposes.forEach((d) => d());
                hot.vfs.delete(vfsKey).then(reload);
              };
              __hot_hmr_callbacks.add(specifier, onChange);
              disposes.push(() => {
                __hot_hmr_callbacks.delete(specifier, onChange);
              });
            }
          });
        };
        __hot_hmr_callbacks.add(url.pathname, reload);
        watchDeps();
      }
    });
  });
}

/** @version: 0.57.7 */

const eventColors = {
  modify: "#056CF0",
  create: "#20B44B",
  remove: "#F00C08",
};

export default {
  name: "hmr",
  setup(hot: any) {
    if (!hot.isDev) {
      return;
    }
    hot.hmr = true;
    hot.hmrModules = new Set<string>();
    hot.hmrCallbacks = new Map<string, (module: any) => void>();
    hot.customImports = {
      ...hot.customImports,
      "@hmrRuntime": "https://esm.sh/hot/_hmr.js",
      "@reactRefreshRuntime": "https://esm.sh/hot/_hmr_react_refresh.js",
    };
    hot.register(
      "_hmr.js",
      () => `
        export default (path) => ({
          decline() {
            HOT.hmrModules.delete(path);
            HOT.hmrCallbacks.set(path, () => location.reload());
          },
          accept(cb) {
            if (!HOT.hmrModules.has(path)) {
              HOT.hmrModules.add(path);
              HOT.hmrCallbacks.set(path, cb);
            }
          },
          invalidate() {
            location.reload();
          }
        })
      `,
      (code: string) => code,
    );
    hot.register(
      "_hmr_react_refresh.js",
      () => `
        // react-refresh
        // @link https://github.com/facebook/react/issues/16604#issuecomment-528663101

        import runtime from "https://esm.sh/v135/react-refresh@0.14.0/runtime";

        let timer;
        const refresh = () => {
          if (timer !== null) {
            clearTimeout(timer);
          }
          timer = setTimeout(() => {
            runtime.performReactRefresh()
            timer = null;
          }, 30);
        };

        runtime.injectIntoGlobalHook(window);
        window.$RefreshReg$ = () => {};
        window.$RefreshSig$ = () => type => type;

        export { refresh as __REACT_REFRESH__, runtime as __REACT_REFRESH_RUNTIME__ };
      `,
      (code: string) => code,
    );
    hot.onFire((_sw: ServiceWorker) => {
      const connect = () => {
        const source = new EventSource(new URL("hot-notify", location.href));
        source.addEventListener("fs-notify", async (ev) => {
          const { type, name } = JSON.parse(ev.data);
          const module = hot.hmrModules.has(name);
          const handler = hot.hmrCallbacks.get(name);
          if (type === "modify") {
            if (module) {
              const url = new URL(name, location.href);
              url.searchParams.set("t", Date.now().toString(36));
              if (url.pathname.endsWith(".css")) {
                url.searchParams.set("module", "");
              }
              const module = await import(url.href);
              if (handler) {
                handler(module);
              }
            } else if (handler) {
              handler();
            }
          }
          if (module || handler) {
            console.log(
              `🔥 %c[HMR] %c${type}`,
              "color:#999",
              `color:${eventColors[type as keyof typeof eventColors]}`,
              `${JSON.stringify(name)}`,
            );
          }
        });
        source.onopen = () => {
          console.log(
            "🔥 %c[HMR]",
            "color:#999",
            "listening for file changes...",
          );
        };
        source.onerror = (err) => {
          if (err.eventPhase === EventSource.CLOSED) {
            console.log(
              "🔥 %c[HMR]",
              "color:#999",
              "connection lost, reconnecting...",
            );
            setTimeout(() => {
              connect();
            }, 300);
          }
        };
      };
      connect();
    });
  },
};

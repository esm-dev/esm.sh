import { setup as setupDevtools } from "./devtools";

declare global {
  interface Window {
    __hot_hmr_modules: Set<string>;
    __hot_hmr_callbacks: Map<string, (module: any) => void>;
  }
}

function setupHMR(hot: any) {
  hot.hmr = true;
  window.__hot_hmr_modules = new Set();
  window.__hot_hmr_callbacks = new Map();

  hot.customImports.set(
    "@hmrRuntime",
    "https://esm.sh/hot/_hmr.js",
  );
  hot.register(
    "_hmr.js",
    () => `
      export default (path) => ({
        decline() {
          __hot_hmr_modules.delete(path);
          __hot_hmr_callbacks.set(path, () => location.reload());
        },
        accept(cb) {
          if (!__hot_hmr_modules.has(path)) {
            __hot_hmr_modules.add(path);
            typeof cb === "function" && __hot_hmr_callbacks.set(path, cb);
          }
        },
        invalidate() {
          location.reload();
        }
      })
    `,
  );

  const logPrefix = ["🔥 %c[HMR]", "color:#999"];
  const eventColors = {
    modify: "#056CF0",
    create: "#20B44B",
    remove: "#F00C08",
  };

  const source = new EventSource(new URL("hot-notify", location.href));
  source.addEventListener("fs-notify", async (ev) => {
    const { type, name } = JSON.parse(ev.data);
    const module = window.__hot_hmr_modules.has(name);
    const callback = window.__hot_hmr_callbacks.get(name);
    if (type === "modify") {
      if (module) {
        const url = new URL(name, location.href);
        url.searchParams.set("t", Date.now().toString(36));
        if (url.pathname.endsWith(".css")) {
          url.searchParams.set("module", "");
        }
        const module = await import(url.href);
        if (callback) {
          callback(module);
        }
      } else if (callback) {
        callback(null);
      }
    }
    if (module || callback) {
      console.log(
        logPrefix[0] + " %c" + type,
        logPrefix[1],
        `color:${eventColors[type as keyof typeof eventColors]}`,
        `${JSON.stringify(name)}`,
      );
    }
  });
  let connected = false;
  source.onopen = () => {
    connected = true;
    console.log(
      ...logPrefix,
      "connected, listening for file changes...",
    );
  };
  source.onerror = (err) => {
    if (err.eventPhase === EventSource.CLOSED) {
      if (!connected) {
        console.warn(...logPrefix, "failed to connect.");
      } else {
        console.log(...logPrefix, "connection lost, reconnecting...");
      }
    }
  };
}

export function setup(hot: any) {
  setupHMR(hot);
  setupDevtools(hot);
}

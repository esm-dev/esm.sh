/*! 🔥 esm.sh/run - speeding up your modern(es2015+) web application with service worker.
 *  Docs: https://docs.esm.sh/run
 */

import type { RunOptions } from "./types/run.d.ts";

const global = globalThis;
const document: Document | undefined = global.document;
const clients: Clients | undefined = global.clients;

function run(options: RunOptions = {}): Promise<ServiceWorker> {
  const serviceWorker = navigator.serviceWorker;
  if (!serviceWorker) {
    throw new Error("Service Worker is restricted to running across HTTPS for security reasons.");
  }
  const hasController = serviceWorker.controller !== null;
  const onUpdateFound = options.onUpdateFound ?? (() => location.reload());
  return new Promise<ServiceWorker>(async (resolve, reject) => {
    const reg = await serviceWorker.register(options.sw ?? "/sw.js", {
      type: "module",
      scope: options.swScope,
    });
    const run = async () => {
      const { active } = reg;
      if (active?.state === "activated") {
        queryElement<HTMLScriptElement>('script[type="importmap"]', (el) => {
          try {
            const { imports } = JSON.parse(el.textContent!);
            if (imports) {
              active.postMessage(["importmap", { imports }]);
            }
          } catch (e) {
            throw new Error("Invalid importmap: " + e.message);
          }
        });
        // import the main module if provided
        if (options.main) {
          import(options.main);
        }
        resolve(active);
      }
    };

    if (hasController) {
      // run the app immediately if the Service Worker is already installed
      run();
      // listen for the new service worker to take over
      serviceWorker.oncontrollerchange = onUpdateFound;
    } else {
      // wait for the new service worker to be installed
      reg.onupdatefound = () => {
        const installing = reg.installing;
        if (installing) {
          installing.onerror = (e) => reject(e.error);
          installing.onstatechange = () => {
            const waiting = reg.waiting;
            if (waiting) {
              waiting.onstatechange = run;
            }
          };
        }
      };
    }
  });
}

function setupServiceWorker() {
  const on = global.addEventListener;
  const importMap: { imports: Record<string, string> } = { imports: {} };

  on("install", (evt) => {
    // @ts-expect-error The `skipWaiting` method of the `ServiceWorkerGlobalScope` interface
    // forces the waiting service worker to become the active service worker.
    skipWaiting();
  });

  on("activate", (evt) => {
    // When a service worker is initially registered, pages won't use it until they next load.
    // The `clients.claim()` method causes those pages to be controlled immediately.
    evt.waitUntil(clients!.claim());
  });

  on("fetch", (evt) => {
    const { request } = evt as FetchEvent;
    if (request.url.startsWith(location.origin)) {
      const url = new URL(request.url);
      const pathname = url.pathname;
      if (/\.(jsx|ts|mts|tsx)$/.test(pathname)) {
      }
    }
  });

  on("message", async (evt) => {
    const { data } = evt;
    if (Array.isArray(data)) {
      const [HEAD] = data;
      if (HEAD === "importmap") {
        importMap.imports = data[1].imports;
      }
    }
  });
}

/** get the attribute value of the given element. */
function attr(el: Element, name: string) {
  return el.getAttribute(name);
}

/** query the element with the given selector and run the callback if found. */
function queryElement<T extends Element>(selector: string, callback: (el: T) => void) {
  const el = document!.querySelector<T>(selector);
  if (el) {
    callback(el);
  }
}

if (document) {
  // run the `main` module if it's provided in the script tag with `src` attribute equals to current script url
  // e.g. <script type="module" src="https://esm.sh/run" main="/main.mjs" sw="/sw.mjs"></script>
  queryElement<HTMLScriptElement>("script[type='module'][src][main]", (el) => {
    const src = el.src;
    const main = attr(el, "main");
    if (src === import.meta.url && main) {
      const options: RunOptions = { main, sw: attr(el, "sw") };
      const updateprompt = attr(el, "updateprompt");
      if (updateprompt) {
        queryElement<HTMLElement>(updateprompt, (el) => {
          options.onUpdateFound = () => {
            el.hidden = false;
            if (el instanceof HTMLDialogElement) {
              el.showModal();
            } else if (el.hasAttribute("popover")) {
              el.showPopover?.();
            }
          };
        });
      }
      run(options);
    }
  });
  // compatibility with esm.sh/run(v1) which has been renamed to 'esm.sh/tsx'
  queryElement<HTMLScriptElement>("script[type^='text/']", () => {
    import(new URL("/tsx", import.meta.url).href);
  });
} else if (clients) {
  setupServiceWorker();
}

export { run, run as default };

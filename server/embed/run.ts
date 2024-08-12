/*! 🔥 esm.sh/run - ts/jsx just works™️ in browser.
 *! 📚 https://docs.esm.sh/run
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
  return new Promise<ServiceWorker>(async (resolve, reject) => {
    const hasController = serviceWorker.controller !== null;
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
          queueMicrotask(() => import(options.main!));
        }
        resolve(active);
      }
    };

    if (hasController) {
      // run the app immediately if the Service Worker is already installed
      run();
      // listen for the new service worker to take over
      serviceWorker.oncontrollerchange = options.onUpdateFound ?? (() => location.reload());
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
  // @ts-expect-error `$TARGET` is injected by esbuild
  const target: string = $TARGET;
  const on = global.addEventListener;
  const importMap: { imports: Record<string, string> } = { imports: {} };
  const regexpTsx = /\.(jsx|ts|mts|tsx)$/;
  const cachePromise = caches.open("esm.sh/run");
  const stringify = JSON.stringify;

  async function tsx(url: URL, code: string) {
    const cache = await cachePromise;
    const filename = url.pathname.split("/").pop()!;
    const extname = filename.split(".").pop()!;
    const buffer = new Uint8Array(
      await crypto.subtle.digest(
        "SHA-1",
        new TextEncoder().encode(extname + code + stringify(importMap) + target + "false"),
      ),
    );
    const id = [...buffer].map((b) => b.toString(16).padStart(2, "0")).join("");
    const cacheKey = new URL(url);
    cacheKey.searchParams.set("_tsxid", id);

    let res = await cache.match(cacheKey);
    if (res) {
      return res;
    }

    res = await fetch(urlFromCurrentModule(`/+${id}.mjs`));
    if (res.status === 404) {
      res = await fetch(urlFromCurrentModule("/transform"), {
        method: "POST",
        body: stringify({ filename, code, importMap, target }),
      });
      const ret = await res.json();
      if (ret.error) {
        throw new Error(ret.error.message);
      }
      res = new Response(ret.code, { headers: { "Content-Type": "application/javascript; charset=utf-8" } });
    }
    if (!res.ok) {
      return res;
    }

    cache.put(cacheKey, res.clone());
    return res;
  }

  on("install", (evt) => {
    // @ts-ignore The `skipWaiting` method forces the waiting service worker to become
    // the active service worker.
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
      if (regexpTsx.test(pathname)) {
        evt.respondWith((async () => {
          const res = await fetch(request);
          if (!res.ok || (/^(text|application)\/javascript/.test(res.headers.get("Content-Type") ?? ""))) {
            return res;
          }
          return tsx(url, await res.text());
        })());
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

/** create a URL object from the given path in the current module. */
function urlFromCurrentModule(path: string) {
  return new URL(path, import.meta.url);
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
    import(urlFromCurrentModule("/tsx").href);
  });
} else if (clients) {
  setupServiceWorker();
}

export { run, run as default };

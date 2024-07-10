/*! 🔥 esm.sh/run - speeding up your modern(es2015+) web application with service worker.
 *  Docs: https://docs.esm.sh/run
 */

import type { RunOptions } from "./types/run.d.ts";

const document: Document | undefined = window.document;
const kRun = "esm.sh/run";

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
      if (reg.active?.state === "activated") {
        let p: Promise<boolean> | undefined;
        let importMap: Record<string, any> | null = null;
        queryElement<HTMLScriptElement>('script[type="importmap"]', (el) => {
          try {
            const json = JSON.parse(el.textContent!);
            if (json && typeof json === "object") {
              importMap = json;
            }
          } catch (e) {
            console.error("Failed to parse importmap:", e);
          }
        });
        queryElement<HTMLLinkElement>("link[rel='preload'][as='fetch'][type='application/esm-bundle'][href]", (el) => {
          p = fetch(el.href).then((res) => {
            if (!res.ok) {
              throw new Error("Failed to download esm-bundle: " + (res.statusText ?? res.status));
            }
            return res.arrayBuffer();
          }).then(async (arrayBuffer) => {
            const checksumAttr = attr(el, "checksum");
            if (checksumAttr) {
              const checksum = btoa(String.fromCharCode(...new Uint8Array(await crypto.subtle.digest("SHA-256", arrayBuffer))));
              if (checksum !== checksumAttr) {
                throw new Error("Invalid esm-bundle: the checksum does not match");
              }
            }
            return new Promise<boolean>((res, rej) => {
              new BroadcastChannel(kRun).onmessage = ({ data }) => {
                if (data === 0) {
                  rej(new Error("Failed to load esm-bundle"));
                } else {
                  res(data === 2);
                }
              };
              reg.active!.postMessage([0x7f, arrayBuffer]);
            });
          });
        });
        if (p) {
          if (hasController) {
            p.then((isStale) => isStale && onUpdateFound());
          } else {
            // if there's no controller(first install), wait for the esm-bundle to be loaded
            await p.catch(reject);
          }
        }
        // import the main module if provided
        if (options.main) {
          import(options.main);
        }
        resolve(reg.active!);
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
          if (el instanceof HTMLDialogElement) {
            el.showModal();
          } else {
            el.hidden = false;
            if (el.hasAttribute("popover")) {
              el.showPopover?.();
            }
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

export { run, run as default };

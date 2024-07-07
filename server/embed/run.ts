/*! 🔥 esm.sh/run - speeding up your modern(es2015+) web application with service worker.
 *  Docs: https://docs.esm.sh/run
 */

import type { RunOptions, VFile } from "./types/run.d.ts";

const document: Document | undefined = window.document;
const kRun = "esm.sh/run";
const kImportmap = "importmap";

async function run(options: RunOptions = {}): Promise<ServiceWorker> {
  const serviceWorker = navigator.serviceWorker;
  const hasController = serviceWorker.controller !== null;
  const {
    main,
    onUpdateFound = () => location.reload(),
    swModule,
    swScope,
  } = options;
  return new Promise<ServiceWorker>(async (resolve, reject) => {
    const reg = await serviceWorker.register(swModule ?? "/sw.js", {
      type: "module",
      scope: swScope,
    });
    const run = async () => {
      if (reg.active?.state === "activated") {
        const importMapSupported = HTMLScriptElement.supports?.(kImportmap);
        const imports: Record<string, string> = {};
        const scopes: Record<string, typeof imports> = {};
        let p: Promise<boolean> | undefined;
        queryElement<HTMLScriptElement>('script[type="importmap"]', (el) => {
          try {
            const json = JSON.parse(el.textContent!);
            for (const scope in json.scopes) {
              scopes[scope] = { ...imports, ...json.imports };
            }
            Object.assign(imports, json.imports);
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
            // if there's no controller, wait for the esm-bundle to be applied
            await p.catch(reject);
          }
        }
        // import the main module if provided
        if (main) {
          import(main);
        }
        resolve(reg.active!);
      }
    };

    // detect Service Worker install/update available and wait for it to become installed
    reg.onupdatefound = () => {
      const installing = reg.installing;
      if (installing) {
        installing.onerror = (e) => reject(e.error);
        installing.onstatechange = () => {
          const waiting = reg.waiting;
          if (waiting) {
            waiting.onstatechange = hasController ? onUpdateFound : run;
          }
        };
      }
    };

    // run the app immediately if the Service Worker is already installed
    if (hasController) {
      run();
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
    run({ main, swModule: attr(el, "sw") ?? undefined });
  }
});

// compatibility with esm.sh/run(v1) which has been renamed to 'esm.sh/tsx'
queryElement<HTMLScriptElement>("script[type^='text/']", () => {
  import(new URL("/tsx", import.meta.url).href);
});

export { run, run as default };

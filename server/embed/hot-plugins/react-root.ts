/** @version: 18.2.0 */

import type { Hot, ImportMap } from "../types/hot.d.ts";

function getImportMap(): ImportMap | null {
  const script = document.querySelector("script[type=importmap]");
  if (script) {
    return JSON.parse(script.textContent!);
  }
  return null;
}

function importAll(...urls: (string | URL)[]) {
  return Promise.all(urls.map((url) => import(url.toString())));
}

export default {
  name: "react-root",
  setup(hot: Hot) {
    hot.onFire((_sw: ServiceWorker) => {
      customElements.define(
        "react-root",
        class ReactRoot extends HTMLElement {
          async connectedCallback() {
            const src = this.getAttribute("src");
            if (!src) {
              return;
            }
            if (hot.isDev) {
              // ensure react-refresh is loaded before react runtime
              await import(
                new URL("/@hot/hmr_react_refresh.js", location.href).href
              );
            }
            const { imports } = getImportMap() ?? {};
            const [
              { createElement, StrictMode },
              { createRoot },
              { default: Component },
            ] = await importAll(
              imports?.["react"] ?? "https://esm.sh/react@18.2.0",
              imports?.["react-dom/client"] ??
                (imports?.["react-dom"] ?? "https://esm.sh/react-dom@18.2.0") +
                  "/client",
              new URL(src, location.href),
            );
            createRoot(this).render(
              createElement(StrictMode, null, createElement(Component)),
            );
          }
        },
      );
    });
  },
};

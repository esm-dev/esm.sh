/** @version: 18.2.0 */

import type { Hot } from "../server/embed/types/hot.d.ts";

function importAll(...urls: (string | URL)[]) {
  return Promise.all(urls.map((url) => import(url.toString())));
}

export default {
  name: "react",
  setup(hot: Hot) {
    hot.onFire(() => {
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
            const { imports } = hot.importMap;
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

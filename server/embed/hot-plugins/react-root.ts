/** @version: 18.2.0 */

function importAll(...urls: (string | URL)[]) {
  return Promise.all(urls.map((url) => import(url.toString())));
}

export default {
  name: "react-root",
  setup(hot: any) {
    hot.onFire((_sw: ServiceWorker) => {
      customElements.define(
        "react-root",
        class ReactRoot extends HTMLElement {
          constructor() {
            super();
          }
          async connectedCallback() {
            const rootDiv = document.createElement("div");
            const src = this.getAttribute("src");
            this.appendChild(rootDiv);
            if (!src) {
              return;
            }
            if (hot.hmr) {
              // ensure react-refresh is injected before react-dom is loaded
              await import("https://esm.sh/hot/_hmr_react_refresh.js");
            }
            const [
              { createElement, StrictMode },
              { createRoot },
              { default: Component },
            ] = await importAll(
              "https://esm.sh/react@18.2.0",
              "https://esm.sh/react-dom@18.2.0/client",
              new URL(src, location.href),
            );
            createRoot(rootDiv).render(
              createElement(StrictMode, null, createElement(Component)),
            );
          }
        },
      );
    });
  },
};

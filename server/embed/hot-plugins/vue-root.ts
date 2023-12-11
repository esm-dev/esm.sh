/** @version: 3.3.9 */

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
  name: "vue-root",
  setup(hot: Hot) {
    hot.onFire((_sw: ServiceWorker) => {
      customElements.define(
        "vue-root",
        class VueRoot extends HTMLElement {
          connectedCallback() {
            const rootDiv = document.createElement("div");
            if (this.hasAttribute("shadow")) {
              const shadow = this.attachShadow({ mode: "open" });
              shadow.appendChild(rootDiv);
            } else {
              this.appendChild(rootDiv);
            }
            const src = this.getAttribute("src");
            if (src) {
              const importMap = getImportMap();
              importAll(
                importMap?.imports?.["vue"] ?? "https://esm.sh/vue@3.3.9",
                new URL(src, location.href),
              ).then(([
                { createApp },
                { default: Component },
              ]) => {
                const app = createApp(Component);
                app.mount(rootDiv);
              });
            }
          }
        },
      );
    });
  },
};

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
            if (this.hasAttribute("shadow") && !this.shadowRoot) {
              this.attachShadow({ mode: "open" });
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
                app.mount(this.shadowRoot ?? this);
              });
            }
          }
        },
      );
    });
  },
};

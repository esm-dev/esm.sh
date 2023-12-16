import type { Hot } from "../server/embed/types/hot.d.ts";

function importAll(...urls: (string | URL)[]) {
  return Promise.all(urls.map((url) => import(url.toString())));
}

export default {
  name: "vue",
  setup(hot: Hot) {
    hot.onFire(() => {
      customElements.define(
        "vue-root",
        class VueRoot extends HTMLElement {
          connectedCallback() {
            if (this.hasAttribute("shadow") && !this.shadowRoot) {
              this.attachShadow({ mode: "open" });
            }
            const src = this.getAttribute("src");
            if (src) {
              const { imports } = hot.importMap;
              importAll(
                imports["vue"] ?? "https://esm.sh/vue@3.3.9",
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

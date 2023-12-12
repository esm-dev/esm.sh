import type { Hot } from "../types/hot.d.ts";

export default {
  name: "svelte-root",
  setup(hot: Hot) {
    hot.onFire((_sw: ServiceWorker) => {
      customElements.define(
        "svelte-root",
        class SvelteRoot extends HTMLElement {
          connectedCallback() {
            if (this.hasAttribute("shadow") && !this.shadowRoot) {
              this.attachShadow({ mode: "open" });
            }
            const src = this.getAttribute("src");
            src &&
              import(new URL(src, location.href).href).then(
                ({ default: Component }) => {
                  new Component({
                    target: this.shadowRoot ?? this,
                  });
                },
              );
          }
        },
      );
    });
  },
};

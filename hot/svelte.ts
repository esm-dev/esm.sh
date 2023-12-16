import type { Hot } from "../server/embed/types/hot.d.ts";

export default {
  name: "svelte",
  setup(hot: Hot) {
    hot.onFire(() => {
      customElements.define(
        "svelte-root",
        class SvelteRoot extends HTMLElement {
          connectedCallback() {
            if (this.hasAttribute("shadow") && !this.shadowRoot) {
              this.attachShadow({ mode: "open" });
            }
            const src = this.getAttribute("src");
            if (src) {
              import(new URL(src, location.href).href).then(
                ({ default: Component }) => {
                  new Component({ target: this.shadowRoot ?? this });
                },
              );
            }
          }
        },
      );
    });
  },
};

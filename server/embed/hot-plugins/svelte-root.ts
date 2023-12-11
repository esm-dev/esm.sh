import type { Hot } from "../types/hot.d.ts";

export default {
  name: "svelte-root",
  setup(hot: Hot) {
    hot.onFire((_sw: ServiceWorker) => {
      customElements.define(
        "svelte-root",
        class SvelteRoot extends HTMLElement {
          connectedCallback() {
            const rootDiv = document.createElement("div");
            if (this.hasAttribute("shadow")) {
              const shadow = this.attachShadow({ mode: "open" });
              shadow.appendChild(rootDiv);
            } else {
              this.appendChild(rootDiv);
            }
            const src = this.getAttribute("src");
            src &&
              import(new URL(src, location.href).href).then(
                ({ default: Component }) => {
                  new Component({
                    target: rootDiv,
                  });
                },
              );
          }
        },
      );
    });
  },
};

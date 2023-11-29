/** @version: 3.3.9 */

import { createApp } from "https://esm.sh/vue@3.3.9";

export default {
  name: "vue-root",
  setup(hot: any) {
    hot.onFire((_sw: ServiceWorker) => {
      customElements.define(
        "vue-root",
        class VueRoot extends HTMLElement {
          constructor() {
            super();
          }
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
              import(new URL(src, location.href).href).then(
                ({ default: Component }) => {
                  const app = createApp(Component);
                  app.mount(rootDiv);
                },
              );
            }
          }
        },
      );
    });
  },
};

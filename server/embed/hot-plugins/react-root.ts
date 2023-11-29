/** @version: 18.2.0 */

import { createElement } from "https://esm.sh/react@18.2.0?dev";
import { createRoot } from "https://esm.sh/react-dom@18.2.0/client?dev";

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
          connectedCallback() {
            const rootDiv = document.createElement("div");
            const src = this.getAttribute("src");
            this.appendChild(rootDiv);
            if (src) {
              import(new URL(src, location.href).href).then(
                ({ default: Component }) => {
                  createRoot(rootDiv).render(createElement(Component));
                },
              );
            }
          }
        },
      );
    });
  },
};

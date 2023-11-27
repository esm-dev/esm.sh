/** @version: 18.2.0 */

import { createElement } from "https://esm.sh/react@18.2.0";
import { createRoot } from "https://esm.sh/react-dom@18.2.0/client";

export default {
  name: "react-root",
  setup(hot: any) {
    if (globalThis.customElements) {
      customElements.define(
        "react-root",
        class MyCustomElement extends HTMLElement {
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
    }
  },
};

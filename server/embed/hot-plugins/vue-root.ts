/** @version: 3.3.9 */

function importAll(...urls: (string | URL)[]) {
  return Promise.all(urls.map((url) => import(url.toString())));
}

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
              importAll(
                "vue",
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

export default {
  name: "svelte-root",
  setup(hot: any) {
    if (globalThis.customElements) {
      customElements.define(
        "svelte-root",
        class MyCustomElement extends HTMLElement {
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
    }
  },
};

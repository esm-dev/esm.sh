const deafultStyle = `
blockquote {
  padding-left: var(--lineHeight);
  border-left: 2px solid #ccc;
}

h1 > a.anchor,
h2 > a.anchor,
h3 > a.anchor,
h4 > a.anchor,
h5 > a.anchor,
h6 > a.anchor {
  display: inline-block;
  float: left;
  height: 1.2em;
  width: 1em;
  margin-left: -1em;
  position: relative;
  outline: none;
}
/*.anchor:target { background: yellow; }*/
h1 > a.anchor:before,
h2 > a.anchor:before,
h3 > a.anchor:before,
h4 > a.anchor:before,
h5 > a.anchor:before,
h6 > a.anchor:before {
  visibility: hidden;
  position: absolute;
  opacity: 0.2;
  right:0;
  top:0;
  width:  1em;
  font-weight:300;
  line-height: inherit;
  content: ""; /* U+E08F */
  text-align: center;
}
h1 > a.anchor:hover:before,
h2 > a.anchor:hover:before,
h3 > a.anchor:hover:before,
h4 > a.anchor:hover:before,
h5 > a.anchor:hover:before,
h6 > a.anchor:hover:before {
  visibility: visible;
  opacity:0.8;
}
h1 > a.anchor:focus:before,
h2 > a.anchor:focus:before,
h3 > a.anchor:focus:before,
h4 > a.anchor:focus:before,
h5 > a.anchor:focus:before,
h6 > a.anchor:focus:before,
h1:hover .anchor:before,
h2:hover .anchor:before,
h3:hover .anchor:before,
h4:hover .anchor:before,
h5:hover .anchor:before,
h6:hover .anchor:before {
  visibility: visible;
}

`;

export default {
  name: "markdown-body",
  setup(hot: any) {
    hot.onActive((_sw: ServiceWorker) => {
      customElements.define(
        "markdown-body",
        class VueRoot extends HTMLElement {
          constructor() {
            super();
          }
          connectedCallback() {
            const rootDiv = document.createElement("div");
            if (this.hasAttribute("shadow")) {
              const shadow = this.attachShadow({ mode: "open" });
              const styleEl = document.createElement("style");
              styleEl.innerHTML = deafultStyle;
              shadow.appendChild(rootDiv);
              shadow.appendChild(styleEl);
            } else {
              this.appendChild(rootDiv);
            }
            const src = this.getAttribute("src");
            if (src) {
              fetch(new URL(src, location.href).href).then(
                (res) => {
                  if (res.ok) {
                    res.text().then((html) => {
                      rootDiv.innerHTML = html;
                    });
                  }
                },
              );
            }
          }
        },
      );
    });
  },
};

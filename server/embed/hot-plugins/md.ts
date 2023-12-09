import { init as initWasm, parse } from "https://esm.sh/markdown-wasm-es@1.2.1";

const deafultStyle = `
h1 > a.anchor,
h2 > a.anchor,
h3 > a.anchor,
h4 > a.anchor,
h5 > a.anchor,
h6 > a.anchor {
  position: relative;
  display: inline-block;
  float: left;
  margin-left: -1em;
  width: 1em;
  height: 1em;
  outline: none;
  color: inherit;
}
h1 > a.anchor:before,
h2 > a.anchor:before,
h3 > a.anchor:before,
h4 > a.anchor:before,
h5 > a.anchor:before,
h6 > a.anchor:before {
  visibility: hidden;
  position: absolute;
  opacity: 0.33;
  right: 0;
  top: 0;
  width: 1em;
  height: 1em;
  content: "⌁";
  line-height: inherit;
  text-align: center;
}
h1 > a.anchor:hover:before,
h2 > a.anchor:hover:before,
h3 > a.anchor:hover:before,
h4 > a.anchor:hover:before,
h5 > a.anchor:hover:before,
h6 > a.anchor:hover:before {
  visibility: visible;
  opacity: 1;
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
  name: "md",
  setup(hot: any) {
    let waiting: Promise<any> | null = null;
    const init = async () => {
      if (waiting === null) {
        waiting = initWasm();
      }
      await waiting;
    };

    hot.onLoad(
      /\.(md|markdown)$/,
      async (_url: URL, source: string, _options: Record<string, any> = {}) => {
        await init();
        const html = parse(source);
        return {
          code: `<style>${deafultStyle}</style>` + html,
          headers: { "content-type": "text/html" },
        };
      },
    );
  },
};

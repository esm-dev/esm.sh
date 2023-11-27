/** @version: 0.3.1 */

import initWasm, {
  transform,
  transformCSS,
} from "https://esm.sh/v135/esm-compiler@0.3.1";

export default {
  name: "tsx",
  setup(hot: any) {
    const { stringify } = JSON;

    let waiting: Promise<any> | null = null;
    const init = async () => {
      if (waiting === null) {
        waiting = initWasm(
          fetch("https://esm.sh/esm-compiler@0.3.1/esm_compiler_bg.wasm"),
        );
      }
      await waiting;
    };

    hot.onLoad(
      /\.(jsx|tsx|ts|css)$/,
      async (url: URL, source: string, options: Record<string, any> = {}) => {
        const { pathname } = url;
        const { isDev, importMap } = options;
        await init();
        if (pathname.endsWith(".css")) {
          const { code, map, exports } = transformCSS(pathname, source, {
            minify: !isDev,
            cssModules: pathname.endsWith(".module.css"),
            targets: {
              chrome: 95 << 16, // TODO: check user agent
            },
            sourceMap: !!isDev,
          });
          if (url.searchParams.has("module")) {
            let css = code;
            if (map) {
              css +=
                "\n//# sourceMappingURL=data:application/json;charset=utf-8;base64,";
              css += btoa(map);
            }
            const cssModulesExports: Record<string, string> = {};
            if (exports) {
              exports.forEach((cssExport, id) => {
                cssModulesExports[id] = cssExport.name;
              });
            }
            return {
              code: [
                "const d = document;",
                "const id = ",
                stringify(pathname),
                ";export const css = ",
                stringify(css),
                ";if (!d.getElementById(id)) {",
                "const style = d.createElement('style');",
                "style.id = id;",
                "style.textContent = css;",
                "d.head.appendChild(style);",
                "}",
                "export default ",
                stringify(cssModulesExports),
              ].join(""),
            };
          }
          return { code, map };
        }
        return transform(pathname, source, {
          isDev,
          sourceMap: !!isDev,
          jsxImportSource: importMap.imports?.["@jsxImportSource"],
          importMap: stringify(importMap),
          minify: !isDev ? { compress: true, keepNames: true } : undefined,
          target: "es2020", // TODO: check user agent
        });
      },
    );
  },
};

/** @version: 0.3.2 */

import initWasm, {
  type Targets,
  transform,
  transformCSS,
} from "https://esm.sh/v135/esm-compiler@0.3.2";

let waiting: Promise<any> | null = null;
const init = async () => {
  if (waiting === null) {
    waiting = initWasm(
      fetch("https://esm.sh/esm-compiler@0.3.2/esm_compiler_bg.wasm"),
    );
  }
  await waiting;
};

export default {
  name: "tsx",
  setup(hot: any) {
    const { stringify } = JSON;

    const targets: Targets = {
      chrome: 95 << 16, // default to chrome 95
    };
    if (!globalThis.document) {
      const { userAgent } = navigator;
      if (userAgent.includes("Safari/")) {
        // safari: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Safari/605.1.15
        // chrome: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36
        let m = userAgent.match(/Version\/(\d+)\.(\d)+/);
        if (m) {
          targets.safari = parseInt(m[1]) << 16 | parseInt(m[2]) << 8;
        } else if ((m = userAgent.match(/Chrome\/(\d+)\./))) {
          targets.chrome = parseInt(m[1]) << 16;
        }
      }
    }

    hot.onLoad(
      /\.(js|mjs|jsx|tsx|ts|css)$/,
      async (url: URL, source: string, options: Record<string, any> = {}) => {
        const { pathname } = url;
        const { importMap } = options;
        const { isDev } = hot;
        await init();
        if (pathname.endsWith(".css")) {
          // todo: check more browsers
          const { code, map, exports } = transformCSS(pathname, source, {
            targets,
            minify: !isDev,
            cssModules: pathname.endsWith(".module.css"),
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
              // TODO: support hmr
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
        const imports = importMap.imports;
        const jsxImportSource = imports?.["@jsxImportSource"];
        const hmrRuntimeUrl = imports?.["@hmrRuntimeUrl"];
        return transform(pathname, source, {
          isDev,
          sourceMap: !!isDev,
          jsxImportSource: jsxImportSource,
          importMap: stringify(importMap ?? {}),
          minify: !isDev ? { compress: true, keepNames: true } : undefined,
          target: "es2020", // TODO: check user agent
          hmr: hmrRuntimeUrl
            ? {
              runtimeUrl: hmrRuntimeUrl,
              reactRefresh: jsxImportSource?.includes("/react"),
              reactRefreshRuntimeUrl: imports?.["@reactRefreshRuntimeUrl"],
            }
            : undefined,
        });
      },
      true, // varyUA
    );
  },
};

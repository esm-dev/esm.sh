import type { Hot } from "../server/embed/types/hot.d.ts";
import initWasm, {
  type Targets,
  transform,
  transformCSS,
} from "https://esm.sh/esm-compiler@0.4.4";

let waiting: Promise<any> | null = null;
const init = async () => {
  if (waiting === null) {
    waiting = initWasm(
      fetch("https://esm.sh/esm-compiler@0.4.4/esm_compiler_bg.wasm"),
    );
  }
  await waiting;
};

export default {
  name: "tsx",
  setup(hot: Hot) {
    const { stringify } = JSON;

    if (hot.isDev) {
      // add `?dev` to react/react-dom import url in development mode
      const isProdReact = (url: URL, req: Request) => {
        if (
          url.hostname === "esm.sh" &&
          !url.searchParams.has("dev") &&
          req.method === "GET"
        ) {
          const p = url.pathname.split("/");
          const [name] = p[1].split("@");
          return p.length <= 3 && (name === "react" || name === "react-dom");
        }
        return false;
      };
      hot.onFetch(isProdReact, (req: Request) => {
        const url = new URL(req.url);
        url.searchParams.set("dev", "");
        return new Response(null, {
          status: 302,
          headers: {
            "access-control-allow-origin": "*",
            "location": url.href.replace("dev=", "dev"),
          },
        });
      });
    }

    hot.onLoad(
      /\.(js|mjs|jsx|mts|ts|tsx|css)$/,
      async (url, source, options) => {
        const { pathname } = url;
        const { importMap, isDev } = options;
        const imports = importMap.imports;
        const hmrRuntime = imports?.["@hmrRuntime"];
        await init();

        if (pathname.endsWith(".css")) {
          const targets: Targets = {
            chrome: 95 << 16, // default to chrome 95
          };
          const { code, map, exports } = transformCSS(pathname, source, {
            targets,
            minify: !isDev,
            cssModules: pathname.endsWith(".module.css"),
            sourceMap: !!isDev,
          });

          if (!url.searchParams.has("module")) {
            return {
              code,
              map,
              contentType: "text/css; charset=utf-8",
            };
          }

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
              isDev && hmrRuntime &&
              `import Hmr from ${stringify(hmrRuntime)};import.meta.hot = Hmr(${
                stringify(pathname)
              });`,
              "const d = document;",
              "const id = ",
              stringify(pathname),
              ";export const css = ",
              stringify(css),
              ";const old = d.getElementById(id);",
              "const style = d.createElement('style');",
              "style.id = id;",
              "style.textContent = css;",
              "d.head.appendChild(style);",
              "old && d.head.removeChild(old);",
              "export default ",
              stringify(cssModulesExports),
              isDev && hmrRuntime &&
              ";import.meta.hot.accept();",
            ].filter(Boolean).join(""),
          };
        }
        const jsxImportSource = imports?.["@jsxImportSource"];
        const reactRefreshRuntime = imports?.["@reactRefreshRuntime"];
        return transform(pathname, source, {
          isDev,
          sourceMap: isDev ? "external" : undefined,
          jsxImportSource: jsxImportSource,
          importMap: importMap.$support ? undefined : stringify(importMap),
          minify: !isDev ? { compress: true, keepNames: true } : undefined,
          target: "es2020",
          hmr: isDev && hmrRuntime
            ? {
              runtime: hmrRuntime,
              reactRefresh: !!reactRefreshRuntime,
              reactRefreshRuntime: reactRefreshRuntime,
            }
            : undefined,
        });
      },
    );
  },
};

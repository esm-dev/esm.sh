import initWasm, { transform } from "https://esm.sh/v135/esm-compiler@0.3.0";

let waiting: Promise<any> | null = null;
const init = async () => {
  if (waiting === null) {
    waiting = initWasm(
      fetch("https://esm.sh/esm-compiler@0.3.0/esm_compiler_bg.wasm"),
    );
  }
  await waiting;
};

export default async (
  url: URL,
  code: string,
  { isDev, importMap }: Record<string, any> = {},
) => {
  await init();
  return transform(url.pathname, code, {
    isDev,
    sourceMap: !!isDev,
    jsxImportSource: importMap.imports?.["@jsxImportSource"],
    importMap: JSON.stringify(importMap),
    minify: !isDev ? { compress: true, keepNames: true } : undefined,
    target: "es2020", // TODO: check user agent
  });
};

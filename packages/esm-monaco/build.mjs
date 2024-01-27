import { build as esbuild } from "esbuild";

const build = (/** @type {import("esbuild").BuildOptions} */ options) => {
  return esbuild({
    target: "esnext",
    format: "esm",
    platform: "browser",
    outdir: "dist",
    bundle: true,
    minify: true,
    logLevel: "info",
    loader: {
      ".ttf": "dataurl",
    },
    ...options,
  });
};

await build({
  entryPoints: [
    "src/editor.ts",
    "src/editor-worker.ts",
    "src/lsp/html/setup.ts",
    "src/lsp/html/worker.ts",
    "src/lsp/css/setup.ts",
    "src/lsp/css/worker.ts",
    "src/lsp/json/setup.ts",
    "src/lsp/json/worker.ts",
    "src/lsp/typescript/setup.ts",
    "src/lsp/typescript/worker.ts",
  ],
});

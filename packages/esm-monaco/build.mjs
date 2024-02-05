import { copyFile, readdir, readFile, writeFile } from "node:fs/promises";
import { build as esbuild } from "esbuild";

const build = (/** @type {string[]} */ entryPoints) => {
  return esbuild({
    target: "esnext",
    format: "esm",
    platform: "browser",
    outdir: "dist",
    bundle: true,
    logLevel: "info",
    loader: {
      ".ttf": "dataurl",
    },
    external: [
      "typescript",
      "*/setup.js",
      "*/libs.js",
      "*/worker.js",
      "*/editor-worker.js",
    ],
    entryPoints,
  });
};
const bundleTypescriptLibs = async () => {
  const dtsFiles = [];
  const libDir = "node_modules/typescript/lib";
  const entries = await readdir(libDir);
  for (const entry of entries) {
    if (entry.startsWith("lib.") && entry.endsWith(".d.ts")) {
      dtsFiles.push(entry);
    }
  }
  const libs = Object.fromEntries(
    await Promise.all(dtsFiles.map(async (name) => {
      return [name, await readFile(libDir + "/" + name, "utf-8")];
    })),
  );
  await writeFile(
    "dist/lsp/typescript/libs.js",
    "export default " + JSON.stringify(libs, undefined, 2),
    "utf-8",
  );
};
const copyDts = (...files) => {
  return Promise.all(files.map(async ([src, dest]) => {
    copyFile("node_modules/" + src, "types/" + dest);
  }));
};

await bundleTypescriptLibs();
await copyDts(
  ["tm-themes/index.d.ts", "tm-themes.d.ts"],
  ["tm-grammars/index.d.ts", "tm-grammars.d.ts"],
  ["monaco-editor-core/esm/vs/editor/editor.api.d.ts", "monaco.d.ts"],
);
await build([
  "src/editor.ts",
  // "src/shiki.ts",
  "src/editor-worker.ts",
  "src/lsp/html/setup.ts",
  "src/lsp/html/worker.ts",
  "src/lsp/css/setup.ts",
  "src/lsp/css/worker.ts",
  "src/lsp/json/setup.ts",
  "src/lsp/json/worker.ts",
  "src/lsp/typescript/setup.ts",
  "src/lsp/typescript/worker.ts",
]);

import { build as esbuild } from "esbuild";
import { copyFile, readdir, readFile, writeFile } from "node:fs/promises";

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

const copyFiles = (...files) => {
  return Promise.all(files.map(async ([src, dest]) => {
    copyFile(src, dest);
  }));
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
    "src/lsp/typescript/api.ts",
  ],
});
await bundleTypescriptLibs();
await copyFiles(
  ["node_modules/tm-themes/index.d.ts", "types/tm-themes.d.ts"],
  ["node_modules/tm-grammars/index.d.ts", "types/tm-grammars.d.ts"],
  ["node_modules/monaco-editor-core/monaco.d.ts", "types/monaco.d.ts"],
);

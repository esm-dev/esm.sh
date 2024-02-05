import { copyFile, readdir, readFile, writeFile } from "node:fs/promises";
import { build as esbuild } from "esbuild";
import { grammars as tmGrammars } from "tm-grammars";
import { themes as tmThemes } from "tm-themes";

// add some aliases for javascript and typescript
const javascriptGrammar = tmGrammars.find((g) => g.name === "javascript");
const typescriptGrammar = tmGrammars.find((g) => g.name === "typescript");
javascriptGrammar.aliases?.push("mjs", "cjs", "jsx");
typescriptGrammar.aliases?.push("mts", "cts", "tsx");

const tmDefine = {
  "TM_THEMES": JSON.stringify(
    tmThemes.map((v) => v.name),
  ),
  "TM_GRAMMARS": JSON.stringify(
    tmGrammars.map((v) => ({ name: v.name, aliases: v.aliases })),
  ),
};
const build = (/** @type {string[]} */ entryPoints, external, define) => {
  return esbuild({
    target: "esnext",
    format: "esm",
    platform: "browser",
    outdir: "dist",
    bundle: true,
    logLevel: "info",
    define,
    loader: {
      ".ttf": "dataurl",
    },
    external: [
      "typescript",
      "*/libs.js",
      "*/worker.js",
      "*/editor-worker.js",
      "*/setup.js",
      ...(external ?? []),
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
const bundleEditorCSS = async () => {
  const css = await readFile("dist/editor.css", "utf-8");
  const js = await readFile("dist/editor.js", "utf-8");
  await writeFile(
    "dist/editor.js",
    "export const _CSS = " + JSON.stringify(css) + "\n" + js,
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
await build(["src/index.ts"], ["*/editor.js"], tmDefine);
await bundleEditorCSS();

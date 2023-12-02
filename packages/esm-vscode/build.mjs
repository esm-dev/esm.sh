import { build as esbuild } from "esbuild";

const build = (options) => {
  return esbuild({
    target: "node18",
    format: "cjs",
    platform: "node",
    outdir: "dist",
    bundle: true,
    minify: false,
    logLevel: "info",
    external: ["vscode", "typescript"],
    ...options,
  });
};

await build({
  entryPoints: ["src/extension.ts"],
});
await build({
  entryPoints: ["src/typescript-esm-plugin.ts"],
  outdir: "typescript-esm-plugin",
});

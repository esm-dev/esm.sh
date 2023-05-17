import { build as esbuild } from "esbuild";

const build = (options) => {
  return esbuild({
    target: "esnext",
    format: "esm",
    platform: "browser",
    outdir: "dist",
    bundle: true,
    external: ["esm-worker"],
    minify: true,
    logLevel: "info",
    outExtension: { ".js": ".mjs" },
    ...options,
  });
};

await build({
  entryPoints: [
    "src/index.ts",
    "src/worker.ts",
  ],
});

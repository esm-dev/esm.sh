import { build as esbuild } from "esbuild";

const build = (options) => {
  return esbuild({
    target: "esnext",
    format: "esm",
    platform: "browser",
    outdir: "dist",
    bundle: true,
    minify: true,
    logLevel: "info",
    ...options,
  });
};

await build({ entryPoints: ["src/compat.ts"] });

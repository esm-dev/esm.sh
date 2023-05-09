import { readFileSync } from "node:fs";
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

const goCode = readFileSync("../../server/consts.go", "utf8");
const [, version] = goCode.match(/VERSION = (\d+)/);
const [, stableVersion] = goCode.match(/STABLE_VERSION = (\d+)/);

if (!version || !stableVersion) {
  throw new Error("Could not find version in consts.go");
}

await build({
  define: {
    "__VERSION__": version,
    "__STABLE_VERSION__": stableVersion,
  },
  entryPoints: ["src/index.ts"],
});

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
const [, stableBuild] = goCode.match(/stableBuild = map\[string\]bool(\{[\s\S]+?\})/);
const [, fixedPkgVersions] = goCode.match(/fixedPkgVersions = map\[string\]string(\{[\s\S]+?\})/);
const [, cssPackages] = goCode.match(/cssPackages = map\[string\]string(\{[\s\S]+?\})/);
const [, assetExts] = goCode.match(/assetExts = map\[string\]bool(\{[\s\S]+?\})/);

function toJson(s) {
 return JSON.stringify(JSON.stringify(JSON.parse(s.split("\n").map((line ) => {
   const [s] = line.split("//")
   return s.trim();
 }).join("\n").replace(/,\n}/g, "\n}"))));
}

await build({
  define: {
    "__VERSION__": version,
    "__STABLE_VERSION__": stableVersion,
    "__STABLE_BUILD__": toJson(stableBuild),
    "__FIXED_PKG_VERSIONS__": toJson(fixedPkgVersions),
    "__CSS_PACKAGES__": toJson(cssPackages),
    "__ASSETS_EXTS__": toJson(assetExts),
  },
  entryPoints: ["src/index.ts"],
});

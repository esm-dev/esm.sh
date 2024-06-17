import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { Readable } from "node:stream";
import { extract } from "/tar@7.2.0/x";

export function __filename$(filename) {
  return path.join(os.homedir(), ".cache/esm.sh", filename);
}

export function __dirname$(dirname) {
  return path.join(os.homedir(), ".cache/esm.sh", dirname);
}

export async function __downloadPackageTarball$(tar) {
  const [registry, pkgName, pkgVersion] = tar.split(" ");
  const savePath = path.join(os.homedir(), ".cache/esm.sh", pkgName + "@" + pkgVersion);
  try {
    await fs.promises.access(path.join(savePath, "package.json"));
    return;
  } catch (error) {
    if (error.code !== "ENOENT") {
      throw error;
    }
  }
  const basenamePrefix = pkgName.startsWith("@") ? pkgName.split("/")[1] : pkgName;
  const url = registry.replace(/\/+$/, "") + `/${pkgName}/-/${basenamePrefix}-${pkgVersion}.tgz`;
  console.log(`Downloading ${url}...`);
  const res = await fetch(url);
  if (!res.ok) {
    throw new Error(`Failed to download package tarball(${url}): ${res.statusText}`);
  }
  const DecompressionStream = globalThis.DecompressionStream || await import("node:stream/web").DecompressionStream;
  const readable = Readable.fromWeb(res.body.pipeThrough(new DecompressionStream("gzip")));
  try {
    // ensure the `savePath` directory exists
    await fs.promises.mkdir(savePath, { recursive: true });
  } catch {
    // ignore
  }
  await new Promise((resolve, reject) => {
    readable.pipe(extract({ C: savePath, strip: 1 })).on("error", reject).on("end", resolve);
  });
}

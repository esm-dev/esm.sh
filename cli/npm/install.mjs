import { chmodSync, createWriteStream, existsSync, linkSync, readFileSync, statSync, unlinkSync } from "node:fs";
import { createRequire } from "node:module";
import { Writable } from "node:stream";

const binExtension = process.platform === "win32" ? ".exe" : "";

// 1. Attempt to resolve "@esm.sh/cli-{os}-{arch}", if not found, then try to download the binary from GitHub.
// 2. On macOS/Linux, link "bin/esm.sh" to "@esm.sh/cli-{os}-{arch}/bin/esm.sh" if exists.
install();

async function install() {
  const binPath = toPackagePath("bin/esm.sh" + binExtension);
  try {
    const nativeBinPath = resolveBinaryPath();
    if (process.platform !== "win32") {
      unlinkSync(binPath);
      linkSync(nativeBinPath, binPath);
      chmodAddX(binPath);
    }
  } catch {
    try {
      console.log("[esm.sh] Trying to download esm.sh binary from GitHub...");
      const readable = await downloadBinaryFromGitHub();
      await readable.pipeTo(Writable.toWeb(createWriteStream(binPath)));
      chmodAddX(binPath);
    } catch (err) {
      console.error("[esm.sh] Failed to install esm.sh binary:", err);
      throw err;
    }
  }
}

function resolveBinaryPath() {
  const cliBinPackage = `@esm.sh/cli-${currentOS()}-${currentArch()}`;
  const binPath = createRequire(import.meta.url).resolve(cliBinPackage + "/bin/esm.sh" + binExtension);
  if (!existsSync(binPath)) {
    throw new Error(`Could not find the binary of '${cliBinPackage}'`);
  }
  return binPath;
}

async function downloadBinaryFromGitHub() {
  const pkgInfo = JSON.parse(readFileSync(toPackagePath("package.json"), "utf8"));
  const [_, minor, patch] = pkgInfo.version.split(".");
  const tag = "v" + minor + (Number(patch) > 0 ? "_" + patch : "");
  const url = `https://github.com/esm-dev/esm.sh/releases/download/${tag}/cli-${currentOS()}-${currentArch()}${binExtension}.gz`;
  const res = await fetch(url);
  if (!res.ok) {
    res.body?.cancel();
    throw new Error(`Download ${url}: <${res.statusText}>`);
  }
  return res.body.pipeThrough(new DecompressionStream("gzip"));
}

function currentOS() {
  switch (process.platform) {
    case "darwin":
      return "darwin";
    case "linux":
      return "linux";
    case "win32":
      return "windows";
    default:
      throw new Error(`Unsupported platform: ${process.platform}`);
  }
}

function currentArch() {
  switch (process.arch) {
    case "arm64":
      return "arm64";
    case "x64":
      return "amd64";
    default:
      throw new Error(`Unsupported architecture: ${process.arch}`);
  }
}

function toPackagePath(filename) {
  return new URL(filename, import.meta.url).pathname;
}

function chmodAddX(path) {
  chmodSync(path, statSync(path).mode | 0o111);
}

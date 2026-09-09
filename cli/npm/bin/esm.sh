#!/usr/bin/env node

const { execFileSync } = require("child_process");
const { chmodSync, createWriteStream, existsSync, readFileSync } = require("fs");
const { Writable } = require("stream");
const { join } = require("path");

// On macOS/Linux, this file will be linked to "@esm.sh/cli-{os}-{arch}/bin/esm.sh" by the install script.
// On Windows, or if the install script is interrupted, the binary path is resolved manually and executed.
try {
  execFileSync(resolveBinaryPath(), process.argv.slice(2), { stdio: "inherit" });
} catch (err) {
  if (typeof err.status === "number" || err.signal) {
    process.exit(err.status || 1);
  }
  downloadBinaryFromGitHub().then(async (res) => {
    const binPath = join(__dirname, "esm.sh" + (getBinExtension() || ".bin"));
    await res.pipeTo(Writable.toWeb(createWriteStream(binPath)));
    chmodSync(binPath, 0o755);
    execFileSync(binPath, process.argv.slice(2), { stdio: "inherit" });
  }).catch((err) => {
    console.error("[esm.sh] Failed to run esm.sh:", err.message);
    process.exit(err.status || 1);
  });
}

function resolveBinaryPath() {
  const exeBinPath = join(__dirname, "esm.sh" + (getBinExtension() || ".bin"));
  if (existsSync(exeBinPath)) {
    return exeBinPath;
  }
  const cliBinPackage = `@esm.sh/cli-${getOS()}-${getArch()}`;
  const binPath = require.resolve(cliBinPackage + "/bin/esm.sh" + getBinExtension());
  if (!existsSync(binPath)) {
    throw new Error(`Could not find the binary of '${cliBinPackage}'`);
  }
  return binPath;
}

async function downloadBinaryFromGitHub() {
  const pkgInfo = JSON.parse(readFileSync(join(__dirname, "../package.json"), "utf8"));
  const tag = "v" + pkgInfo.version;
  const url = `https://github.com/esm-dev/esm.sh/releases/download/${tag}/cli-${getOS()}-${getArch()}${getBinExtension()}.gz`;
  const res = await fetch(url);
  if (!res.ok) {
    res.body?.cancel();
    throw new Error(`Download ${url}: <${res.statusText}>`);
  }
  return res.body.pipeThrough(new DecompressionStream("gzip"));
}

function getOS() {
  switch (process.platform) {
    case "darwin":
    case "linux":
    case "win32":
      return process.platform;
    default:
      throw new Error(`Unsupported platform: ${process.platform}`);
  }
}

function getArch() {
  switch (process.arch) {
    case "arm64":
    case "x64":
      return process.arch;
    default:
      throw new Error(`Unsupported architecture: ${process.arch}`);
  }
}

function getBinExtension() {
  return process.platform === "win32" ? ".exe" : "";
}

#!/usr/bin/env node

const { execFileSync } = require("child_process");
const { createWriteStream, existsSync, readFileSync } = require("fs");
const { Writable } = require("stream");
const { join } = require("path");

// On macOS/Linux, this file will be linked to "@esm.sh/cli-{os}-{arch}/bin/esm.sh" by the install script.
// On Windows, or if the install script is interrupted, the binary path is resolved manually and executed.
try {
  execFileSync(resolveBinaryPath(), process.argv.slice(2), { stdio: 'inherit' })
} catch (err) {
  downloadBinaryFromGitHub().then((res) => {
    const binPath = join(__dirname, "esm.sh" + (getBinExtension() || ".bin"));
    res.pipeTo.pipeTo(Writable.toWeb(createWriteStream(binPath))).then(() => {
      execFileSync(binPath, process.argv.slice(2), { stdio: 'inherit' });
    });
  });
}

function resolveBinaryPath() {
  const exeBinPath = join(__dirname, "esm.sh.exe");
  if (existsSync(exeBinPath)) {
    return exeBinPath;
  }
  const cliBinPackage = `@esm.sh/cli-${getPlatform()}-${getArch()}`;
  const binPath = require.resolve(cliBinPackage + "/bin/esm.sh" + getBinExtension());
  if (!existsSync(binPath)) {
    throw new Error(`Could not find the binary of '${cliBinPackage}'`);
  }
  return binPath;
}

async function downloadBinaryFromGitHub() {
  const pkgInfo = JSON.parse(readFileSync(join(__dirname, "../package.json"), "utf8"));
  const version = pkgInfo.version.split(".")[1];
  const url = `https://github.com/esm-dev/esm.sh/releases/download/v${version}/esm.sh-cli-${getPlatform()}-${getArch()}.gz`;
  const res = await fetch(url);
  if (!res.ok) {
    res.body?.cancel();
    throw new Error(`Download ${url}: <${res.statusText}>`);
  }
  return res.body.pipeThrough(new DecompressionStream("gzip"));
}

function getPlatform() {
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

function getArch() {
  switch (process.arch) {
    case "arm64":
      return "arm64";
    case "x64":
      return "amd64";
    default:
      throw new Error(`Unsupported architecture: ${process.arch}`);
  }
}

function getBinExtension() {
  return process.platform === "win32" ? ".exe" : "";
}

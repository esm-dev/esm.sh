import { chmodSync, cpSync, createWriteStream, existsSync, linkSync, mkdirSync, readFileSync, rmSync, statSync } from "node:fs";
import { createRequire } from "node:module";
import { arch, platform } from "node:os";
import { Writable } from "node:stream";

async function install() {
  const binPath = toPackagePath("bin/esm.sh" + getBinExtension());
  if (!existsSync(binPath)) {
    try {
      // ensure bin directory exists
      mkdirSync(toPackagePath("bin"));
    } catch {}
    try {
      if (platform() !== "win32") {
        linkSync(await resolveBinaryPath(), binPath);
        // chmod +x
        chmodSync(binPath, statSync(binPath).mode | 0o111);
      } else {
        cpSync(await resolveBinaryPath(), binPath);
      }
    } catch {
      try {
        const fileStream = createWriteStream(binPath);
        console.log("Downloading esm.sh CLI binary from GitHub...");
        await downloadBinaryFromGitHub(fileStream);
      } catch (err) {
        console.error("Failed to install esm.sh CLI binary:", err);
        rmSync(toPackagePath("bin"), { recursive: true });
        process.exit(1);
      }
    }
  }
}

async function resolveBinaryPath() {
  const cliBinPackage = `@esm.sh/cli-${getPlatform()}-${getArch()}`;
  const binPath = createRequire(import.meta.url).resolve(cliBinPackage + "/bin/esm.sh" + getBinExtension());
  if (!existsSync(binPath)) {
    throw new Error(`Package '${cliBinPackage}' may be installed incorrectly`);
  }
  return binPath;
}

async function downloadBinaryFromGitHub(w) {
  const pkgInfo = JSON.parse(readFileSync(toPackagePath("package.json"), "utf8"));
  const version = pkgInfo.version.split(".")[1];
  const url = `https://github.com/esm-dev/esm.sh/releases/download/v${version}/esm.sh-cli-${getPlatform()}-${getArch()}.gz`;
  const res = await fetch(url);
  if (!res.ok) {
    res.body?.cancel?.();
    throw new Error(`Download ${url}: <${res.statusText}>`);
  }
  await res.body.pipeThrough(new DecompressionStream("gzip")).pipeTo(Writable.toWeb(w));
}

function getPlatform() {
  let os = platform();
  os = os === "win32" ? "windows" : os;
  switch (os) {
    case "darwin":
      return "darwin";
    case "linux":
      return "linux";
    case "win32":
      return "windows";
    default:
      throw new Error(`Unsupported platform: ${os}`);
  }
}

function getArch() {
  const a = arch();
  switch (a) {
    case "arm64":
      return "arm64";
    case "x64":
      return "amd64";
    default:
      throw new Error(`Unsupported architecture: ${a}`);
  }
}

function getBinExtension() {
  return platform() === "win32" ? ".exe" : "";
}

function toPackagePath(filename) {
  return new URL(filename, import.meta.url).pathname;
}

install();

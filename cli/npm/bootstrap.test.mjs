import { expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { runInNewContext } from "node:vm";

const source = readFileSync(new URL("bin/esm.sh", import.meta.url), "utf8");

for (const platform of ["darwin", "linux", "win32"]) {
  for (const arch of ["arm64", "x64"]) {
    test(`bootstrap ${platform}-${arch}`, async () => {
      const extension = platform === "win32" ? ".exe" : "";
      const calls = [];
      let installed = false;
      const modules = {
        child_process: { execFileSync: (path, args) => calls.push(["exec", path, args]) },
        fs: {
          existsSync: () => installed,
          readFileSync: () => JSON.stringify({ version: "0.1.1" }),
          createWriteStream: (path) => { calls.push(["write", path]); return {}; },
          chmodSync: (path, mode) => calls.push(["chmod", path, mode]),
        },
        stream: { Writable: { toWeb: (stream) => stream } },
        path: { join: (...parts) => parts.join("/") },
      };
      const require = Object.assign((id) => modules[id], {
        resolve: (id) => { calls.push(["resolve", id]); throw new Error("missing optional package"); },
      });
      const context = {
        require,
        __dirname: "/package/bin",
        process: { platform, arch, argv: ["node", "esm.sh", "--version"], exit: (code) => { throw new Error(`exit ${code}`); } },
        console,
        DecompressionStream: class {},
        fetch: async (url) => {
          calls.push(["fetch", url]);
          return { ok: true, body: { pipeThrough: () => ({ pipeTo: async () => { installed = true; } }) } };
        },
      };
      await runInNewContext(source, context);
      const binPath = "/package/bin/esm.sh" + (extension || ".bin");
      expect(calls).toContainEqual(["resolve", `@esm.sh/cli-${platform}-${arch}/bin/esm.sh${extension}`]);
      expect(calls).toContainEqual(["fetch", `https://github.com/esm-dev/esm.sh/releases/download/v0.1.1/cli-${platform}-${arch}${extension}.gz`]);
      expect(calls).toContainEqual(["chmod", binPath, 0o755]);
      expect(calls).toContainEqual(["exec", binPath, ["--version"]]);
      calls.length = 0;
      await runInNewContext(source, { ...context });
      expect(calls).toEqual([["exec", binPath, ["--version"]]]);
    });
  }
}

test("bootstrap preserves the native command exit status", () => {
  const require = Object.assign((id) => ({
    child_process: { execFileSync: () => { throw Object.assign(new Error("failed"), { status: 23 }); } },
    fs: { existsSync: () => true },
    stream: {},
    path: { join: (...parts) => parts.join("/") },
  })[id], { resolve: () => "/native" });
  expect(() => runInNewContext(source, {
    require,
    __dirname: "/package/bin",
    process: { platform: "linux", arch: "x64", argv: [], exit: (code) => { throw new Error(`exit ${code}`); } },
  })).toThrow("exit 23");
});

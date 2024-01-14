import { homedir } from "node:os";
import { createWriteStream } from "node:fs";
import { mkdir, readFile } from "node:fs/promises";
import { Writable } from "node:stream";
import { basename, dirname, join } from "node:path";

export async function loadWasm(wasmUrl, imports) {
  const cachePath = join(homedir(), ".cache", wasmUrl.slice("https://".length));

  let wasmInstance;
  try {
    const bytes = await readFile(cachePath);
    const wasmModule = new WebAssembly.Module(bytes);
    wasmInstance = new WebAssembly.Instance(wasmModule, imports);
  } catch (err) {
    if (err.code !== "ENOENT" && !(err instanceof WebAssembly.CompileError)) throw err;
  }

  if (!wasmInstance) {
    console.log(`Installing ${basename(wasmUrl)}...`);
    const res = await fetch(wasmUrl);
    if (!res.ok) throw new Error(`unexpected response ${res.statusText}`);
    const [body, bodyCopy] = res.body.tee();
    const cachDir = dirname(cachePath);
    await mkdir(cachDir, { recursive: true });
    const writable = createWriteStream(cachePath);
    await Promise.all([
      bodyCopy.pipeTo(Writable.toWeb(writable)),
      WebAssembly.instantiateStreaming(
        new Response(body, { headers: res.headers }),
        imports,
      ).then((res) => {
        wasmInstance = res.instance;
      }),
    ]);
  }

  return wasmInstance;
}

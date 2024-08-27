// @ts-expect-error $TARGET is defined at build time
import { init, transform } from "/esm-compiler@0.7.2/$TARGET/esm_compiler.mjs";
const wasmPromise = loadWasm("/esm-compiler@0.7.2/pkg/esm_compiler_bg.wasm");

async function loadWasm(url: string) {
  // up to 3 attempts in case of network failure
  for (let i = 1; i <= 3; i++) {
    try {
      await init(new URL(url, import.meta.url));
      break;
    } catch (err) {
      if (i < 3) {
        await new Promise((r) => setTimeout(r, i * 100));
      } else {
        console.error(err);
      }
    }
  }
}

export async function tsx(
  pathname: string,
  code: string,
  importMap: { imports?: Record<string, string> },
  target: string,
): Promise<Response> {
  try {
    await wasmPromise;
    const ret = transform(pathname, code, { importMap, target });
    return new Response(ret.code, { headers: { "Content-Type": "application/javascript; charset=utf-8" } });
  } catch (err) {
    return new Response(err.message, { status: 500 });
  }
}

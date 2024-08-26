// @ts-expect-error $TARGET is defined at build time
import { init, transform } from "/esm-compiler@0.6.2/$TARGET/esm_compiler.mjs";
const initPromise = init("/esm-compiler@0.6.2/pkg/esm_compiler_bg.wasm");

export async function tsx(
  url: URL,
  code: string,
  importMap: { imports?: Record<string, string> },
  target: string,
  cachePromise: Promise<Cache>,
): Promise<Response> {
  await initPromise;
  const ret = transform(url.pathname, code, { importMap, target });
  return new Response(ret.code, { headers: { "Content-Type": "application/javascript; charset=utf-8" } });
}

import { assertEquals } from "jsr:@std/assert";

Deno.test("issue #602", async (t) => {
  // await t.step("?target=deno", async () => {
  //   const zstd = await import("http://localhost:8080/@bokuweb/zstd-wasm@0.0.20");
  //   await zstd.init();
  //   const encoder = new TextEncoder();
  //   const buffer = encoder.encode("Hello World");
  //   const compressed = zstd.compress(buffer, 10);
  //   const decompressed = zstd.decompress(compressed);
  //   const decoder = new TextDecoder();
  //   assertEquals(decoder.decode(decompressed), "Hello World");
  // });
  await t.step("?target=browser", async () => {
    const zstd = await import("http://localhost:8080/@bokuweb/zstd-wasm@0.0.20?target=es2022");
    await zstd.init("http://localhost:8080/@bokuweb/zstd-wasm@0.0.20/dist/esm/wasm/zstd.wasm");
    const encoder = new TextEncoder();
    const buffer = encoder.encode("Hello World");
    const compressed = zstd.compress(buffer, 10);
    const decompressed = zstd.decompress(compressed);
    const decoder = new TextDecoder();
    assertEquals(decoder.decode(decompressed), "Hello World");
  });
});

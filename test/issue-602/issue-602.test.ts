import { assertEquals } from "https://deno.land/std@0.180.0/testing/asserts.ts";

import {
  compress,
  decompress,
  init,
} from "http://localhost:8080/@bokuweb/zstd-wasm@0.0.20";

Deno.test("issue #602", async () => {
  await init("http://localhost:8080/@bokuweb/zstd-wasm@0.0.20/dist/esm/wasm/zstd.wasm");
  const encoder = new TextEncoder();
  const buffer = encoder.encode("Hello World");
  const compressed = compress(buffer, 10);
  const decompressed = decompress(compressed);
  const decoder = new TextDecoder();
  assertEquals(decoder.decode(decompressed), "Hello World");
});

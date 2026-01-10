import { assert, assertEquals } from "jsr:@std/assert";

Deno.test("issue #578", async () => {
  const res = await fetch(`http://localhost:8080/katex@0.16.4/dist/katex.mjs?target=esnext`);
  res.body?.cancel();
  const esmPathHeader = res.headers.get("X-Esm-Path");
  assertEquals(esmPathHeader, "/katex@0.16.4/esnext/katex.mjs");
  const tsHeader = res.headers.get("x-typescript-types");
  assert(tsHeader?.startsWith("http://localhost:8080/@types/katex@~0.16"));
  assert(tsHeader?.endsWith("/index.d.ts"));
});

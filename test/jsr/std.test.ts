import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("@std/encoding", async () => {
  const { decodeBase64, encodeBase64 } = await import("https://esm.sh/jsr/@std/encoding@1.0.0/base64");
  assertEquals(encodeBase64("hello"), "aGVsbG8=");
  assertEquals(new TextDecoder().decode(decodeBase64("aGVsbG8=")), "hello");
});

Deno.test("jsr subpath", async () => {
  const res = await fetch("http://localhost:8080/jsr/@std/assert@1.0.10/mod.ts");
  assertEquals(res.status, 200);
  assertEquals(res.headers.get("x-typescript-types"), "http://localhost:8080/@jsr/std__assert@1.0.10/_dist/mod.d.ts");
  assertStringIncludes(await res.text(), "/@jsr/std__assert@1.0.10/denonext/mod.ts.mjs");
});

import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("issue #1285 - Export path ending with .map fails to resolve", async () => {
  const res = await fetch(
    "http://localhost:8080/es-iterator-helpers@1.2.2/Iterator.prototype.map",
    { headers: { "User-Agent": "i'm a browser" } },
  );
  const js = await res.text();
  assertEquals(res.status, 200);
  assertEquals(res.headers.get("content-type"), "application/javascript; charset=utf-8");
  assertStringIncludes(js, 'export * from "/es-iterator-helpers@1.2.2/es2022/Iterator.prototype.map.mjs"');
});

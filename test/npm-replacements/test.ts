import { assert, assertEquals } from "jsr:@std/assert";

Deno.test("npm replacements", async () => {
  const res = await fetch("http://localhost:8080/get-intrinsic@1.2.4/es2022/get-intrinsic.mjs");
  const code = await res.text();
  assertEquals(res.status, 200);
  assertEquals(res.headers.get("cache-control"), "public, max-age=31536000, immutable");
  assertEquals(res.headers.get("content-type"), "application/javascript; charset=utf-8");
  assert(!res.headers.get("vary")!.includes("User-Agent"));
  assert(!code.includes("import")); // should not have import statements
});

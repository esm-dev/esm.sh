import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("issue #1288 - support `with { type: 'json' }`", async () => {
  const res = await fetch(
    "http://localhost:8080/@uppy/dashboard@5.1.0/es2022/dashboard.mjs",
    { redirect: "follow" },
  );
  assertEquals(res.ok, true, "should be found");
  const js = await res.text();
  assertStringIncludes(js, '/@uppy/dashboard@5.1.0/package.json?module"', "Should contain package.json?module");
});

import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

// related issue: https://github.com/esm-dev/esm.sh/issues/896
Deno.test("issue #896", async () => {
  const res = await fetch("http://localhost:8080/redux-first-router-link@2.1.1&deps=redux-first-router@2.1.1/");
  assertEquals(res.status, 200);
  assertEquals(res.headers.get("content-type"), "application/javascript; charset=utf-8");
  const text = await res.text();
  assertStringIncludes(text, "redux-first-router@2.1.1");
});

import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

// https://github.com/esm-dev/esm.sh/issues/1099
Deno.test("fix #1099", async () => {
  const res = await fetch("http://localhost:8080/gh/jeff-hykin/quik-router@aebcafb/main/main.js");
  assertEquals(res.status, 200);
  assertStringIncludes(await res.text(), `"/gh/jeff-hykin/quik-router@aebcafb/denonext/quik-router.mjs"`);
});

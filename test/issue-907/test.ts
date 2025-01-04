import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("issue #907", async () => {
  const res = await fetch("http://localhost:8080/browserslist-generator@3.0.0/denonext/browserslist-generator.mjs");
  assertEquals(res.status, 200);
  assertStringIncludes(await res.text(), `from"/@mdn/browser-compat-data@^5.6.2/data.json"with{type:"json"}`);
});

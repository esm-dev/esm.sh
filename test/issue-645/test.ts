import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("issue #645", async () => {
  const res = await fetch("http://localhost:8080/node-releases@2.0.12/deno/data/release-schedule/release-schedule.json.mjs");
  assertEquals(res.status, 200);
  assertEquals(res.headers.get("content-type")!, "application/javascript; charset=utf-8");
  assertStringIncludes(await res.text(), "export default ");
});

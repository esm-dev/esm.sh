import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("node polyfill", async () => {
  {
    const res = await fetch("http://localhost:8080/node/process.mjs");
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("content-type"), "application/javascript; charset=utf-8");
    assertStringIncludes(await res.text(), `exit:`);
  }
});

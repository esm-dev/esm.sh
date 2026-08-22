import { assertEquals } from "jsr:@std/assert";

Deno.test("issue #1388 - ignore source map exports during analysis", async () => {
  const res = await fetch(
    "http://localhost:8080/vanilla-jsoneditor@3.13.0/standalone.js",
    { headers: { "User-Agent": "i'm a browser" } },
  );
  await res.body?.cancel();

  assertEquals(res.status, 200);
  assertEquals(
    res.headers.get("content-type"),
    "application/javascript; charset=utf-8",
  );
});

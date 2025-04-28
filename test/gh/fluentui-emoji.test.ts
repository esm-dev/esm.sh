import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("github assets", async () => {
  const res = await fetch(
    "http://localhost:8080/gh/microsoft/fluentui-emoji@62ecdc0/assets/Alien/Flat/alien_flat.svg",
  );
  assertEquals(res.status, 200);
  assertEquals(res.headers.get("content-type"), "image/svg+xml; charset=utf-8");
  assertStringIncludes(await res.text(), "<svg");
});

import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("Transfrom SFC on fly", async () => {
  {
    const res = await fetch("http://localhost:8080/gh/phosphor-icons/vue@v2.2.0/src/icons/PhAirplay.vue?deps=vue@3.5.8", {
      headers: { "user-agent": "i'm a browser" },
    });
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("content-type"), "application/javascript; charset=utf-8");
    const code = await res.text();
    assertStringIncludes(code, "/vue@3.5.8/");
    assertStringIncludes(code, "/PhAirplay.vue.js");
  }
});

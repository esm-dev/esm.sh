import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("typesVersions", async () => {
  {
    const res = await fetch("http://localhost:8080/@redux-saga/core@1.3.0");
    res.body?.cancel();
    assertEquals(res.headers.get("x-typescript-types"), "http://localhost:8080/@redux-saga/core@1.3.0/types/ts4.2/index.d.ts");
  }
  {
    const res = await fetch("http://localhost:8080/web-streams-polyfill@3.3.3");
    res.body?.cancel();
    assertEquals(res.headers.get("x-typescript-types"), "http://localhost:8080/web-streams-polyfill@3.3.3/dist/types/ts3.6/polyfill.d.ts");
  }
});

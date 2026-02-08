import { assert, assertEquals, assertStringIncludes } from "jsr:@std/assert";

import * as tslib from "http://localhost:8080/tslib?exports=__await,__spread";

Deno.test("?exports", () => {
  assertEquals(Object.keys(tslib), ["__await", "__spread"]);
});

Deno.test("?exports no imports", async () => {
  {
    const res = await fetch("http://localhost:8080/effect@3.19.16?exports=Effect", { headers: { "User-Agent": "i'm a browser" } });
    assertEquals(res.status, 200);
    const text = await res.text();
    assert(!text.includes('import "'));
    assertStringIncludes(text, 'export * from "/effect@3.19.16/es2022/effect.mjs?exports=Effect"');
  }
  {
    const res = await fetch("http://localhost:8080/effect@3.19.16/es2022/effect.mjs?exports=Effect", {
      headers: { "User-Agent": "i'm a browser" },
    });
    assertEquals(res.status, 200);
    const text = await res.text();
    assertEquals(text.trim(), 'import*as r from"./Effect.mjs";export{r as Effect};');
  }
});

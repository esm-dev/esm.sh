import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

// close https://github.com/esm-dev/esm.sh/issues/1145
Deno.test("issue #1145", async () => {
  {
    const res = await fetch("http://localhost:8080/*gh/shareup/signal-utils@655ae6c/mod.ts?target=esnext", {
      headers: { "user-agent": "i'm a browser" },
    });
    assertEquals(res.status, 200);
    assertStringIncludes(await res.text(), `export * from "/*gh/shareup/signal-utils@655ae6c/esnext/mod.ts.mjs"`);
  }
  {
    const res = await fetch("http://localhost:8080/*gh/shareup/signal-utils@655ae6c/esnext/mod.ts.mjs", {
      headers: { "user-agent": "i'm a browser" },
    });
    assertEquals(res.status, 200);
    assertStringIncludes(await res.text(), `from"@preact/signals-core"`);
  }
});

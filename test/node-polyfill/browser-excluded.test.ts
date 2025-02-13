import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

/**
  `browser` field in package.json is used to exclude some node.js specific modules from the browser build.
  ```json
  {
    "browser": {
      "node:buffer": false
    }
  }
  ```
*/
Deno.test("browser excluded node polyfill", async () => {
  {
    const res = await fetch("http://localhost:8080/cbor-x@1.6.0/es2022/cbor-x.mjs", {
      headers: { "user-agent": "i'm a browser" },
    });
    assertEquals(res.status, 200);
    assertStringIncludes(await res.text(), `const __Buffer$ = globalThis.Buffer;`);
  }
});

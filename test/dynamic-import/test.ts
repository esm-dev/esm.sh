import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("dynamic-import", async () => {
  const res = await fetch("http://localhost:8080/esm-monaco@0.0.0-beta.11/lsp/html/setup?target=es2022");
  res.body?.cancel();
  assertEquals(res.status, 200);

  const esmId = res.headers.get("x-esm-path");
  const res2 = await fetch(`http://localhost:8080/${esmId}`);
  assertEquals(res.status, 200);

  const code = await res2.text();
  assertStringIncludes(code, `from"../language-features.js"`);
  assertStringIncludes(code, `import("./worker.js")`);
});

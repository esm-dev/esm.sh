import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("?bundle", async () => {
  const res = await fetch("http://localhost:8080/buffer@6.0.3?bundle&target=es2022");
  res.body?.cancel();
  assertEquals(res.headers.get("x-esm-path")!, "/buffer@6.0.3/es2022/buffer.bundle.mjs");
  const res2 = await fetch(new URL(res.headers.get("x-esm-path")!, "http://localhost:8080"));
  assertStringIncludes(await res2.text(), `"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"`);
});

Deno.test("?bundle with ?external", async () => {
  const res = await fetch("http://localhost:8080/ajv@8.12.0?bundle&external=fast-deep-equal");
  res.body?.cancel();
  assertStringIncludes(res.headers.get("x-esm-path")!, "/X-ZWZhc3QtZGVlcC1lcXVhbA/");
  assertStringIncludes(res.headers.get("x-esm-path")!, "/ajv.bundle.mjs");
  const res2 = await fetch(new URL(res.headers.get("x-esm-path")!, "http://localhost:8080"));
  assertStringIncludes(await res2.text(), `from"fast-deep-equal"`);
});

Deno.test("?bundle=false", async () => {
  const res = await fetch("http://localhost:8080/@pyscript/core@0.3.4/dist/py-terminal-XWbSa71s?bundle=false&target=es2022");
  res.body?.cancel();
  assertEquals(res.headers.get("x-esm-path")!, "/@pyscript/core@0.3.4/es2022/dist/py-terminal-XWbSa71s.nobundle.js");
  const res2 = await fetch(new URL(res.headers.get("x-esm-path")!, "http://localhost:8080"));
  const code = await res2.text();
  assertStringIncludes(code, "./core.nobundle.js");
  assertStringIncludes(code, "./error-96hMSEw8.nobundle.js");
});

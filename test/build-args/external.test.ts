import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("`?external` query", async () => {
  const res1 = await fetch("http://localhost:8080/react-dom@18.3.1?external=react");
  const code1 = await res1.text();
  assertStringIncludes(code1, '"/react-dom@18.3.1/X-ZXJlYWN0/denonext/react-dom.mjs"');

  const res2 = await fetch("http://localhost:8080/*preact@10.23.2/jsx-runtime");
  const code2 = await res2.text();
  assertStringIncludes(code2, '"/preact@10.23.2/X-Kg/denonext/jsx-runtime.js"');

  const res3 = await fetch("http://localhost:8080/preact@10.23.2/hooks?external=preact");
  const code3 = await res3.text();
  assertStringIncludes(code3, '"/preact@10.23.2/X-ZXByZWFjdA/denonext/hooks.js"');
});

Deno.test("drop invalid `?external`", async () => {
  const res1 = await fetch("http://localhost:8080/react-dom@18.3.1?target=es2022&external=foo,bar,react");
  const code1 = await res1.text();
  assertStringIncludes(code1, '"/react-dom@18.3.1/X-ZXJlYWN0/es2022/react-dom.mjs"');

  const res2 = await fetch("http://localhost:8080/react-dom@18.3.1?target=es2022&external=foo,bar,preact");
  const code2 = await res2.text();
  assertStringIncludes(code2, '"/react-dom@18.3.1/es2022/react-dom.mjs"');

  const res3 = await fetch("http://localhost:8080/react-dom@18.3.1?external=react-dom");
  const code3 = await res3.text();
  assertStringIncludes(code3, '"/react-dom@18.3.1/denonext/react-dom.mjs"');
});

Deno.test("types with `?external`", async () => {
  const res = await fetch("http://localhost:8080/swr@1.3.0/X-ZXJlYWN0/dist/use-swr.d.ts");
  assertEquals(res.status, 200);
  assertEquals(res.headers.get("Content-type"), "application/typescript; charset=utf-8");
  const ts = await res.text();
  assertStringIncludes(ts, '/// <reference types="react" />');
  assertStringIncludes(ts, 'import("react")');
});

Deno.test("external nodejs internal modules", async () => {
  const res = await fetch("http://localhost:8080/cheerio@0.22.0/es2022/cheerio.mjs");
  assertEquals(res.status, 200);
  assertStringIncludes(await res.text(), ` from "/node/buffer.js"`);

  const res2 = await fetch("http://localhost:8080/cheerio@0.22.0?target=es2022&external=node:buffer");
  res2.body?.cancel();
  assertEquals(res2.status, 200);
  const res3 = await fetch("http://localhost:8080/" + res2.headers.get("x-esm-path"));
  assertEquals(res3.status, 200);
  assertStringIncludes(await res3.text(), ` from "node:buffer"`);

  const res4 = await fetch("http://localhost:8080/*cheerio@0.22.0?target=es2022");
  res4.body?.cancel();
  assertEquals(res4.status, 200);
  const res5 = await fetch("http://localhost:8080/" + res4.headers.get("x-esm-path"));
  assertEquals(res5.status, 200);
  assertStringIncludes(await res5.text(), ` from "node:buffer"`);
});

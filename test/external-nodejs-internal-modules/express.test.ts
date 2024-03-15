import { assertEquals, assertStringIncludes } from "https://deno.land/std@0.220.0/assert/mod.ts";

Deno.test("external-nodejs-internal-modules", async () => {
  const res = await fetch("http://localhost:8080/cheerio@0.22.0/es2022/cheerio.mjs");
  assertEquals(res.status, 200);
  assertStringIncludes(await res.text(), ` from "/node/buffer.js"`);

  const res2 = await fetch("http://localhost:8080/cheerio@0.22.0?target=es2022&external=node:buffer");
  res2.body?.cancel();
  assertEquals(res2.status, 200);
  const res3 = await fetch("http://localhost:8080/" + res2.headers.get("x-esm-id"));
  assertEquals(res3.status, 200);
  assertStringIncludes(await res3.text(), ` from "node:buffer"`);

  const res4 = await fetch("http://localhost:8080/*cheerio@0.22.0?target=es2022");
  res4.body?.cancel();
  assertEquals(res4.status, 200);
  const res5 = await fetch("http://localhost:8080/" + res4.headers.get("x-esm-id"));
  assertEquals(res5.status, 200);
  assertStringIncludes(await res5.text(), ` from "node:buffer"`);
});

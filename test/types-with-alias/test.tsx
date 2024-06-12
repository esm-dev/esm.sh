import { assertEquals, assertStringIncludes } from "https://deno.land/std@0.220.0/assert/mod.ts";

Deno.test("types with ?alias", async () => {
  const res = await fetch(
    `http://localhost:8080/@emotion/styled@11.11.5/X-YXJlYWN0OnByZWFjdC9jb21wYXQKZHByZWFjdEAxMC42LjY/types/base.d.ts`,
  );
  assertEquals(res.status, 200);
  assertEquals(res.headers.get("Content-type"), "application/typescript; charset=utf-8");
  const ts = await res.text();
  assertStringIncludes(ts, "preact@10.6.6/compat/src/index.d.ts");
});

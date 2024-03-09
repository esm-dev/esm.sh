// fixes https://github.com/esm-dev/esm.sh/issues/165

import { assertEquals, assertStringIncludes } from "https://deno.land/std@0.210.0/testing/asserts.ts";

Deno.test("issue #165", async () => {
  const res = await fetch("http://localhost:8080/@react-three/fiber@8.15.19?deps=react@18.1.0");
  assertEquals(res.status, 200);
  const code = await res.text();
  assertStringIncludes(code, "/react@18.1.0/");
});

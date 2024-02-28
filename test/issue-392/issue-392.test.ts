import { assertEquals } from "https://deno.land/std@0.210.0/testing/asserts.ts";

import mod from "http://localhost:8080/@rollup/plugin-commonjs@11.1.0";

Deno.test("issue #392", () => {
  assertEquals(typeof mod, "function");
});

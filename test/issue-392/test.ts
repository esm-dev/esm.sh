import { assertEquals } from "jsr:@std/assert";

import mod from "http://localhost:8080/@rollup/plugin-commonjs@11.1.0";

Deno.test("issue #392", () => {
  assertEquals(typeof mod, "function");
});

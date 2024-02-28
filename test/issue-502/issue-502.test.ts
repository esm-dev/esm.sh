import { assertEquals } from "https://deno.land/std@0.210.0/testing/asserts.ts";

import d from "http://localhost:8080/d@1.0.1";

Deno.test("issue #502", () => {
  assertEquals(typeof d, "function");
});

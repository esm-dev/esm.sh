import { assertEquals } from "https://deno.land/std@0.178.0/testing/asserts.ts";

import { BigInteger } from "http://localhost:8080/jsbn@1.1.0";

Deno.test("issue #548", () => {
  assertEquals(typeof BigInteger, "function");
});

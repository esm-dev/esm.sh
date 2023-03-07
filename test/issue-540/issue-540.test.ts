import { assertEquals } from "https://deno.land/std@0.178.0/testing/asserts.ts";

import { isImportSpecifier } from "http://localhost:8080/@babel/types@7.21.2";

Deno.test("issue #540", () => {
  assertEquals(typeof isImportSpecifier, "function");
});

import { assertEquals } from "https://deno.land/std@0.210.0/testing/asserts.ts";

import { exists } from "http://localhost:8080/@kwsites/file-exists@1.1.1";

Deno.test("issue #611", async () => {
  assertEquals(typeof exists, "function");
});

import { assertEquals } from "https://deno.land/std@0.170.0/testing/asserts.ts";

import compareVersions from "http://localhost:8080/tiny-version-compare@3.0.1";

Deno.test("tiny-version-compare", async () => {
  assertEquals(compareVersions("1.12.0", "v1.12.0"), 0);
});

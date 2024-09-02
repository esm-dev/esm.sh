import { assertEquals } from "jsr:@std/assert";

import { exists } from "http://localhost:8080/@kwsites/file-exists@1.1.1";

Deno.test("issue #611", async () => {
  assertEquals(typeof exists, "function");
});

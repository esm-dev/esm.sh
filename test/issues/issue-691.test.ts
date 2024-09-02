import { assertEquals } from "jsr:@std/assert";

import extractFiles from "http://localhost:8080/extract-files@12.0.0/extractFiles.mjs";

Deno.test("issue #691", () => {
  assertEquals(typeof extractFiles, "function");
});

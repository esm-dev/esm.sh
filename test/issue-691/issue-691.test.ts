import { assertEquals } from "https://deno.land/std@0.220.0/assert/mod.ts";

import extractFiles from "http://localhost:8080/extract-files@12.0.0/extractFiles.mjs";

Deno.test("issue #691", () => {
  assertEquals(typeof extractFiles, "function");
});

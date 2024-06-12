import { assertEquals } from "https://deno.land/std@0.220.0/assert/mod.ts";

import { decodeHTML } from "http://localhost:8080/entities@4.4.0/lib/decode";

Deno.test("issue #503", () => {
  assertEquals(typeof decodeHTML, "function");
});

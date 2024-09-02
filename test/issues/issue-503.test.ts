import { assertEquals } from "jsr:@std/assert";

import { decodeHTML } from "http://localhost:8080/entities@4.4.0/lib/decode";

Deno.test("issue #503", () => {
  assertEquals(typeof decodeHTML, "function");
});

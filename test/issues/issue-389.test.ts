import { assertEquals } from "jsr:@std/assert";

import { Value } from "http://localhost:8080/@sinclair/typebox@0.24.27/value";

Deno.test("issue #389", () => {
  assertEquals(typeof Value, "object");
  assertEquals(typeof Value.Check, "function");
});

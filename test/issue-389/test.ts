import { assertEquals } from "https://deno.land/std@0.220.0/assert/mod.ts";

import { Value } from "http://localhost:8080/@sinclair/typebox@0.24.27/value";

Deno.test("issue #389", () => {
  assertEquals(typeof Value, "object");
  assertEquals(typeof Value.Check, "function");
});

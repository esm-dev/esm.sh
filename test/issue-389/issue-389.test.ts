import { equal } from "https://deno.land/std@0.145.0/testing/asserts.ts";

import { Value } from "http://localhost:8080/@sinclair/typebox@0.24.27/value?dev";

Deno.test("issue #389", () => {
  equal(typeof Value, "function");
});

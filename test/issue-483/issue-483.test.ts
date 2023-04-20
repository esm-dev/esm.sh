import { assertEquals } from "https://deno.land/std@0.180.0/testing/asserts.ts";

import { useDrag } from "http://localhost:8080/@use-gesture/react@10.2.24";

Deno.test("issue #483", async () => {
  assertEquals(typeof useDrag, "function");
});

import { assertEquals } from "https://deno.land/std@0.210.0/testing/asserts.ts";

import { Mark } from "http://localhost:8080/@observablehq/plot@0.6.13";

Deno.test("issue #808", async () => {
  assertEquals(typeof Mark.prototype.plot, "function");
});

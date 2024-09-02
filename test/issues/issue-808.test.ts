import { assertEquals } from "jsr:@std/assert";

import { Mark } from "http://localhost:8080/@observablehq/plot@0.6.13";

Deno.test("issue #808", async () => {
  assertEquals(typeof Mark.prototype.plot, "function");
});

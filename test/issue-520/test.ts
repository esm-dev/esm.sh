import { assertEquals } from "https://deno.land/std@0.220.0/assert/mod.ts";

import Draggable from "http://localhost:8080/react-draggable@4.4.5";

Deno.test("issue #520", () => {
  assertEquals(typeof Draggable, "function");
});

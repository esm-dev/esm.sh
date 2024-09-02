import { assertEquals } from "jsr:@std/assert";

import Draggable from "http://localhost:8080/react-draggable@4.4.5";

Deno.test("issue #520", () => {
  assertEquals(typeof Draggable, "function");
});

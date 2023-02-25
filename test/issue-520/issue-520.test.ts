import { assertEquals } from "https://deno.land/std@0.178.0/testing/asserts.ts";

import Draggable from "http://localhost:8080/react-draggable";

Deno.test("issue #520", () => {
  assertEquals(typeof Draggable, "function");
});

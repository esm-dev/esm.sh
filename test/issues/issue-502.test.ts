import { assertEquals } from "jsr:@std/assert";

import d from "http://localhost:8080/d@1.0.1";

Deno.test("issue #502", () => {
  assertEquals(typeof d, "function");
});

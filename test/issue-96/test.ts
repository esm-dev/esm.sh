import { assertEquals } from "jsr:@std/assert";

import { any } from "http://localhost:8080/prop-types@15.7.2?dev";

Deno.test("issue #96 (prop-types development exports)", () => {
  assertEquals(typeof any, "function");
  assertEquals(typeof any.isRequired, "function");
});

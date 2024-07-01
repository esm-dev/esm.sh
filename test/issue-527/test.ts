import { assertEquals } from "jsr:@std/assert";

import { blue } from "http://localhost:8080/@twind/preset-tailwind@1.1.4/colors";

Deno.test("issue #527", () => {
  assertEquals(typeof blue, "object");
});

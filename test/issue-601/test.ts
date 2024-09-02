import { assertEquals } from "jsr:@std/assert";

import { Location } from "http://localhost:8080/@aws-sdk/client-location@3.48.0";

Deno.test("issue #601", async () => {
  assertEquals(typeof Location, "function");
});

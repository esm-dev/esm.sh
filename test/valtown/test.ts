import { assertEquals, assertExists } from "jsr:@std/assert";

import ValTown from "http://localhost:8080/@valtown/sdk@0.30.0";

Deno.test("valtown SDK", () => {
  assertEquals(typeof ValTown, "function");
  const valtown = new ValTown({ bearerToken: "My Bearer Token" });
  assertExists(valtown, "emails");
});

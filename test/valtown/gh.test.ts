import { assertEquals, assertExists } from "jsr:@std/assert";

import ValTown from "http://localhost:8080/gh/val-town/sdk@v0.31.0/src/index.ts";

Deno.test("import valtown SDK from Github", () => {
  assertEquals(typeof ValTown, "function");
  const valtown = new ValTown({ bearerToken: "My Bearer Token" });
  assertExists(valtown, "emails");
});

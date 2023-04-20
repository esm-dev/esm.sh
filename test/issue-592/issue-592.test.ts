import { assertEquals } from "https://deno.land/std@0.180.0/testing/asserts.ts";

import { createConnection } from "http://localhost:8080/mysql2@3.2.0";

Deno.test("issue #592", async () => {
  assertEquals(typeof createConnection, "function");
});

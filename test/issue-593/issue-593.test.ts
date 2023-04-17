import { assertEquals } from "https://deno.land/std@0.178.0/testing/asserts.ts";

import { eventChannel } from "http://localhost:8080/redux-saga@1.2.0";

Deno.test("issue #593", async () => {
  assertEquals(typeof eventChannel, "function");
});

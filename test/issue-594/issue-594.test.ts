import { assertEquals } from "https://deno.land/std@0.178.0/testing/asserts.ts";

import { isLinkButton } from "http://localhost:8080/discord-api-types@0.37.37/utils/v10";

Deno.test("issue #594", async () => {
  assertEquals(typeof isLinkButton, "function");
});

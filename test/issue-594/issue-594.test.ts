import { assertEquals } from "https://deno.land/std@0.210.0/testing/asserts.ts";

import { isLinkButton } from "http://localhost:8080/discord-api-types@0.37.37/utils/v10";

Deno.test("issue #594", () => {
  assertEquals(typeof isLinkButton, "function");
});

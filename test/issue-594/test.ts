import { assertEquals } from "jsr:@std/assert";

import { isLinkButton } from "http://localhost:8080/discord-api-types@0.37.37/utils/v10";

Deno.test("issue #594", () => {
  assertEquals(typeof isLinkButton, "function");
});

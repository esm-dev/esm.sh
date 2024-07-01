import { assertEquals } from "jsr:@std/assert";

import { Expo } from "http://localhost:8080/expo-server-sdk@3.7.0";

Deno.test("issue #566", () => {
  assertEquals(typeof Expo, "function");
});

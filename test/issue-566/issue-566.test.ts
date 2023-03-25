import { assertEquals } from "https://deno.land/std@0.178.0/testing/asserts.ts";

import { Expo } from "http://localhost:8080/expo-server-sdk@3.7.0";

Deno.test("issue #566", () => {
  assertEquals(typeof Expo, "function");
});

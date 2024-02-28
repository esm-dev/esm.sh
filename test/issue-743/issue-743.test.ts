import { assertEquals } from "https://deno.land/std@0.210.0/testing/asserts.ts";

import OpenAI from "http://localhost:8080/openai@4.12.4";

Deno.test("issue #743", () => {
  assertEquals(typeof OpenAI, "function");
});

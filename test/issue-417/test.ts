import { assertEquals } from "https://deno.land/std@0.220.0/assert/mod.ts";

import { ReadableStream } from "http://localhost:8080/web-streams-ponyfill";

Deno.test("issue #417", () => {
  const readable = new ReadableStream();
  assertEquals(typeof readable, "object");
});

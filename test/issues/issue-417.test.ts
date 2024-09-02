import { assertEquals } from "jsr:@std/assert";

import { ReadableStream } from "http://localhost:8080/web-streams-ponyfill";

Deno.test("issue #417", () => {
  const readable = new ReadableStream();
  assertEquals(typeof readable, "object");
});

import { assertEquals } from "jsr:@std/assert";

import { streamParse } from "http://localhost:8080/gh/bablr-lang/bablr@279a549d0bf730e7d8ed008386ff157fc5b0fecd/lib/enhanceable.mjs";

Deno.test("issue #761", () => {
  assertEquals(typeof streamParse, "function");
});

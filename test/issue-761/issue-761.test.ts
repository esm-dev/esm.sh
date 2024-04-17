import { assertEquals } from "https://deno.land/std@0.220.0/assert/mod.ts";

import { streamParse } from "http://localhost:8080/gh/bablr-lang/bablr@279a549d0bf730e7d8ed008386ff157fc5b0fecd/lib/enhanceable.js";

Deno.test("issue #761", () => {
  assertEquals(typeof streamParse, "function");
});

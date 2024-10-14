import { assertEquals } from "jsr:@std/assert";

import type { HLJSApi } from "http://localhost:8080/highlight.js@11.9.0";
import hljs from "http://localhost:8080/highlight.js@11.9.0/lib/core";

// https://github.com/esm-dev/esm.sh/issues/888
Deno.test("issue-888", () => {
  assertEquals(typeof (hljs satisfies HLJSApi), "object");
  assertEquals(typeof hljs.highlight, "function");
});

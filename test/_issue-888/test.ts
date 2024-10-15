import { assertEquals } from "jsr:@std/assert";

import * as hljs from "http://localhost:8080/highlight.js@11.9.0";

// https://github.com/esm-dev/esm.sh/issues/888
Deno.test("issue-888", () => {
  console.log(hljs);
});

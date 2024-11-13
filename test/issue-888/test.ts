import { assertEquals } from "jsr:@std/assert";

import hljs from "http://localhost:8080/highlight.js@11.9.0";

// https://github.com/esm-dev/esm.sh/issues/888
Deno.test("issue-888", () => {
  const html = hljs.highlight(
    "<span>Hello World!</span>",
    { language: "xml" },
  ).value;
  assertEquals(
    html,
    '<span class="hljs-tag">&lt;<span class="hljs-name">span</span>&gt;</span>Hello World!<span class="hljs-tag">&lt;/<span class="hljs-name">span</span>&gt;</span>',
  );
});

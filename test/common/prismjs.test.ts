import { assert, assertEquals } from "jsr:@std/assert";

import Prism from "http://localhost:8080/prismjs@1.29.0";
import "http://localhost:8080/prismjs@1.29.0/components/prism-bash";

Deno.test("prismjs", async () => {
  const code = `var data = 1;`;
  const html = Prism.highlight(code, Prism.languages.javascript, "javascript");
  assertEquals(
    html,
    `<span class="token keyword">var</span> data <span class="token operator">=</span> <span class="token number">1</span><span class="token punctuation">;</span>`,
  );
  assert(Object.keys(Prism.languages).includes("bash"));
});

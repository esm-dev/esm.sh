import { assert } from "https://deno.land/std@0.178.0/testing/asserts.ts";

import Prism from "http://localhost:8080/prismjs";
import "http://localhost:8080/prismjs/components/prism-bash";

Deno.test("prismjs", async () => {
  const code = `var data = 1;`;
  const html = Prism.highlight(code, Prism.languages.javascript, "javascript");
  assert(
    html ===
      `<span class="token keyword">var</span> data <span class="token operator">=</span> <span class="token number">1</span><span class="token punctuation">;</span>`,
  );
  assert(Object.keys(Prism.languages).includes("bash"));
});

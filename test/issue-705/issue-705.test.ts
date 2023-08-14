import { assertStringIncludes } from "https://deno.land/std@0.180.0/testing/asserts.ts";

import { getHighlighter } from "http://localhost:8080/shikiji@0.3.3";

Deno.test("issue #705", async () => {
  const shiki = await getHighlighter({
    themes: ["nord", "min-light"],
    langs: ["javascript"],
  });

  const code = shiki.codeToHtmlDualThemes('console.log("hello")', {
    lang: "javascript",
    themes: {
      light: "min-light",
      dark: "nord",
    },
  });
  assertStringIncludes(
    code,
    `<pre class="shiki shiki-dual-themes min-light nord`,
  );
});

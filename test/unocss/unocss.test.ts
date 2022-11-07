import { assert } from "https://deno.land/std@0.162.0/testing/asserts.ts";

import { createGenerator } from "http://localhost:8080/@unocss/core@0.45";
import presetUno from "http://localhost:8080/@unocss/preset-uno@0.45";
import presetIcons from "http://localhost:8080/@unocss/preset-icons@0.45";
import carbonIcons from "http://localhost:8080/@iconify-json/carbon@1.1/icons.json" assert {
  type: "json",
};

const html = `
<div class="p-2 m-2 flex items-center justify-center"></div>
<span class="text-gray-600 i-carbon-logo-github"></span>
<span class="i-carbon-nonono"></span>
`;

Deno.test("unocss", async () => {
  const uno = createGenerator({
    presets: [
      presetUno(),
      presetIcons({
        collections: {
          carbon: () => carbonIcons,
        },
      }),
    ],
  });
  const { css } = await uno.generate(html, { id: "index.html" });
  assert(css.includes(".p-2"));
  assert(css.includes(".m-2"));
  assert(css.includes(".flex"));
  assert(css.includes(".items-center"));
  assert(css.includes(".justify-center"));
  assert(css.includes(".text-gray-600"));
  assert(css.includes(".i-carbon-logo-github"));
  assert(!css.includes(".i-carbon-nonono"));
});

import { assertStringIncludes } from "https://deno.land/std@0.220.0/assert/mod.ts";

import { compile } from "http://localhost:8080/svelte@4.2.15/compiler";

const appSvelte = `
<script>
import { onMount } from "svelte"
let name = 'world';
onMount(() => {
  console.log('mounted');
})
</script>
<h1>Hello {name}!</h1>
`;

Deno.test("svelte compiler", async () => {
  const ret = compile(
    appSvelte,
    {
      filename: "App.svelte",
      generate: "dom",
      sveltePath: "https://esm.sh/svelte",
    },
  );

  assertStringIncludes(ret.js.code, "App.svelte");
  assertStringIncludes(ret.js.code, `import { onMount } from "https://esm.sh/svelte"`);
  assertStringIncludes(ret.js.code, `"https://esm.sh/svelte/internal"`);
  assertStringIncludes(ret.js.code, "console.log('mounted')");
  assertStringIncludes(ret.js.code, `element("h1")`);
  assertStringIncludes(ret.js.code, "textContent = `Hello ");
});

Deno.test("svelte compiler(SSR)", async () => {
  const ret = compile(
    appSvelte,
    {
      filename: "App.svelte",
      generate: "ssr",
      sveltePath: "https://esm.sh/svelte",
    },
  );

  assertStringIncludes(ret.js.code, "App.svelte");
  assertStringIncludes(ret.js.code, `import { onMount } from "https://esm.sh/svelte"`);
  assertStringIncludes(ret.js.code, `"https://esm.sh/svelte/internal"`);
  assertStringIncludes(ret.js.code, "console.log('mounted')");
  assertStringIncludes(ret.js.code, "return `<h1>Hello ");
});

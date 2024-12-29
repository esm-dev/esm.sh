import { assertStringIncludes } from "jsr:@std/assert";

import { compile } from "http://localhost:8080/svelte@5.16.0/compiler";

const appSvelte = `
<script>
import { onMount } from "svelte"
let name = $state('world');
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
      generate: "client",
    },
  );

  assertStringIncludes(ret.js.code, `import { onMount } from "svelte"`);
  assertStringIncludes(ret.js.code, `"svelte/internal/client"`);
  assertStringIncludes(ret.js.code, "console.log('mounted')");
  assertStringIncludes(ret.js.code, "let name = 'world'");
  assertStringIncludes(ret.js.code, "template(`<h1> </h1>`)");
  assertStringIncludes(ret.js.code, "`Hello ");
});

Deno.test("svelte compiler(SSR)", async () => {
  const ret = compile(
    appSvelte,
    {
      filename: "App.svelte",
      generate: "server",
    },
  );

  assertStringIncludes(ret.js.code, `import { onMount } from "svelte"`);
  assertStringIncludes(ret.js.code, `"svelte/internal/server"`);
  assertStringIncludes(ret.js.code, "let name = 'world'");
  assertStringIncludes(ret.js.code, "console.log('mounted')");
  assertStringIncludes(ret.js.code, "`<h1>Hello ");
});

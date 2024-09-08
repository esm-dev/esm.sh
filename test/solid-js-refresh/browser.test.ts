import { assertStringIncludes } from "jsr:@std/assert";

import { transformAsync } from "http://localhost:8080/@babel/core@^7.24.7?target=es2022";
import babelPresetTS from "http://localhost:8080/@babel/preset-typescript@^7.24.7?target=es2022";
import babelPresetSolid from "http://localhost:8080/babel-preset-solid@1.6.12?target=es2022";
import solidRefresh from "http://localhost:8080/solid-refresh@0.5.2/babel?target=es2022";

Deno.test("babel with solid plugins", async () => {
  const code = `
    import { createSignal, type Component } from "solid-js";
    const Foo: Component = () => {
      return <h1>Foo</h1>;
    }
  `;
  const result = await transformAsync(code, {
    presets: [
      [babelPresetTS, { onlyRemoveTypeImports: true }],
      [babelPresetSolid, { generate: "ssr" }],
    ],
    plugins: [[solidRefresh, { bundler: "vite" }]],
    filename: "example.tsx",
  });
  assertStringIncludes(result!.code as string, `from "solid-js/web"`);
  assertStringIncludes(result!.code as string, `from "solid-refresh"`);
  assertStringIncludes(result!.code as string, `const _tmpl$ = "<h1>Foo</h1>"`);
  assertStringIncludes(result!.code as string, "if (import.meta.hot) {");
});

import { assertStringIncludes } from "https://deno.land/std@0.180.0/testing/asserts.ts";

import { transformAsync } from "http://localhost:8080/@babel/core@7.21.3";
import babelPresetTS from "http://localhost:8080/@babel/preset-typescript@7.21.0";
import babelPresetSolid from "http://localhost:8080/babel-preset-solid@1.6.12";
import solidRefresh from "http://localhost:8080/solid-refresh@0.5.2/babel";

Deno.test("babel/core", async () => {
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
  assertStringIncludes(result!.code as string, `if (import.meta.hot) {`);
});

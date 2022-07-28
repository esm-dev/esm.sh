import { equal } from "https://deno.land/std@0.145.0/testing/asserts.ts";

import { transform } from "http://localhost:8080/@babel/standalone@7.18.9";
import babelPresetSolid from "http://localhost:8080/babel-preset-solid@1.4.6";
import { renderToString } from "http://localhost:8080/solid-js@1.4.7/web";

function transformSolid(rawCode: string): string {
  const { code } = transform(rawCode, {
    presets: [
      [babelPresetSolid, {
        generate: "ssr",
        hydratable: false,
      }],
      ["typescript", { onlyRemoveTypeImports: true }],
    ],
    filename: "main.jsx",
  });
  if (!code) {
    throw new Error("code is empty");
  }
  return code.replaceAll(`"solid-js"`, `"http://localhost:8080/solid-js@1.4.7"`)
    .replaceAll(`"solid-js/web"`, `"http://localhost:8080/solid-js@1.4.7/web"`);
}

Deno.test("solid.js ssr", async () => {
  const code = `import { createSignal } from "solid-js";

  function Counter() {
    const [count, setCount] = createSignal(0);
    const increment = () => setCount(count() + 1);

    return (
      <button type="button" onClick={increment}>
        {count()}
      </button>
    );
  }

  export default function App() {
    return <Counter />;
  }
  `;
  const { default: App } = await import(
    `data:application/javascript,${encodeURIComponent(transformSolid(code))}`
  );
  const html = renderToString(App);
  equal(html, "<button>0</button>");
});

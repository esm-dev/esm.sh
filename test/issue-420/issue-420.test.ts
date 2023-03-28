import {
  assertStringIncludes,
} from "https://deno.land/std@0.178.0/testing/asserts.ts";

import { html } from "http://localhost:8080/htm/preact?deps=preact@10.11.3";
import { useState } from "http://localhost:8080/preact@10.11.3/hooks";
import renderToString from "http://localhost:8080/preact-render-to-string@5.2.0?deps=preact@10.11.3";

Deno.test("issue #420", () => {
  function App() {
    const [count, setCount] = useState(0);

    return html`<div>
      <p>${count}</p>
      <button onClick=${() => setCount(count + 1)}>click</button>
    </div>`;
  }
  assertStringIncludes(renderToString(html`<${App} />`), "<p>0</p>");
});

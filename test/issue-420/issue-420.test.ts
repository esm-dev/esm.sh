import {
  assertStringIncludes,
} from "https://deno.land/std@0.162.0/testing/asserts.ts";

import { render } from "http://localhost:8080/preact-render-to-string?deps=preact@10.11.3";
import { html } from "http://localhost:8080/htm/preact?deps=preact@10.11.3";
import { useState } from "http://localhost:8080/preact@10.11.3/hooks";

Deno.test("issue #420", () => {
  function App() {
    const [count, setCount] = useState(0);

    return html`<div>
      <p>${count}</p>
      <button onClick=${() => setCount(count + 1)}>click</button>
    </div>`;
  }
  assertStringIncludes(render(html`<${App} />`), "<p>0</p>");
});

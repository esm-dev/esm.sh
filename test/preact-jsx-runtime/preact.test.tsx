/** @jsxImportSource http://localhost:8080/preact */

import { assert } from "https://deno.land/std@0.178.0/testing/asserts.ts";

import { useState } from "http://localhost:8080/preact/hooks";
import render from "http://localhost:8080/preact-render-to-string";

Deno.test("preact-jsx-runtime", () => {
  const App = () => {
    const [message] = useState("Hi :)");
    return (
      <main>
        <h1>{message}</h1>
      </main>
    );
  };
  const html = render(<App />);
  assert(html == "<main><h1>Hi :)</h1></main>");
});

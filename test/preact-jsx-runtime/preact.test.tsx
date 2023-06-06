import { assert } from "https://deno.land/std@0.180.0/testing/asserts.ts";

import { useState } from "http://localhost:8080/preact@10.14.0/hooks";
import render from "http://localhost:8080/preact-render-to-string@6.0.3?deps=preact@10.14.0";

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

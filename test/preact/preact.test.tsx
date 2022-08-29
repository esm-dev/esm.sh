import { assertEquals } from "https://deno.land/std@0.145.0/testing/asserts.ts";

import { h } from "http://localhost:8080/preact";
import { useState } from "http://localhost:8080/preact/hooks";
import render from "http://localhost:8080/preact-render-to-string";

Deno.test("preact", () => {
  const App = () => {
    const [message] = useState("Hi :)");
    return (
      <main>
        <h1>{message}</h1>
      </main>
    );
  };
  const html = render(<App />);
  assertEquals(html, "<main><h1>Hi :)</h1></main>");
});

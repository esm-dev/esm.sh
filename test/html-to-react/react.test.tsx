import { assert } from "https://deno.land/std@0.180.0/testing/asserts.ts";

import React from "http://localhost:8080/react@18?dev";
import { renderToString } from "http://localhost:8080/react-dom@18/server";
import { Parser } from "http://localhost:8080/html-to-react?dev&deps=react@18";

Deno.test("html-to-react", async () => {
  const h = new Parser();
  const App = () => {
    return h.parse(`<h1>Hi :)</h1>`);
  };
  const html = renderToString(<App />);
  assert(typeof html === "string" && html.includes("<h1>Hi :)</h1>"));
});

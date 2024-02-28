import { assert } from "https://deno.land/std@0.210.0/testing/asserts.ts";

import React from "http://localhost:8080/react@18.2.0";
import { renderToString } from "http://localhost:8080/react-dom@18.2.0/server";
import { Parser } from "http://localhost:8080/html-to-react@1.5.0?deps=react@18.2.0";

Deno.test("html-to-react", () => {
  const h = new Parser();
  const App = () => {
    return h.parse(`<h1>Hi :)</h1>`);
  };
  const html = renderToString(<App />);
  assert(typeof html === "string" && html.includes("<h1>Hi :)</h1>"));
});

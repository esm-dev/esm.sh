import { assert } from "https://deno.land/std@0.145.0/testing/asserts.ts";

import React from "http://localhost:8080/react@18";
import { renderToString } from "http://localhost:8080/react-dom@18/server";

Deno.test("react", async () => {
  const html = renderToString(
    <main>
      <h1>Hi :)</h1>
    </main>,
  );
  assert(
    typeof html === "string" &&
      html.includes("<h1>Hi :)</h1>") &&
      html.includes("<main") && html.includes("</main>"),
  );
});

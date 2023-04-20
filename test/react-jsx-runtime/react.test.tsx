import {
  assert,
  assertStringIncludes,
} from "https://deno.land/std@0.180.0/testing/asserts.ts";

import { renderToString } from "http://localhost:8080/react-dom@18.2.0/server";

Deno.test("react-jsx-runtime", async () => {
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
  const res = await fetch(
    "http://localhost:8080/stable/react@18.2.0/deno/jsx-runtime.js",
  );
  assertStringIncludes(
    await res.text(),
    `"/stable/react@18.2.0/deno/react.mjs"`,
  );
});

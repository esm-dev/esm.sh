import { assert, assertStringIncludes } from "jsr:@std/assert";

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
    "http://localhost:8080/react@18.2.0/esnext/jsx-runtime.js",
  );
  assertStringIncludes(await res.text(), `"/react@18.2.0/esnext/react.mjs"`);
});

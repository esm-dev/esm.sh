import { assert } from "https://deno.land/std@0.130.0/testing/asserts.ts";

import { renderToString } from "http://localhost:8080/react-dom@18/server";

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
});

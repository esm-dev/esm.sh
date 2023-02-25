import { assertStringIncludes } from "https://deno.land/std@0.178.0/testing/asserts.ts";

import React from "http://localhost:8080/react@18";
import { renderToReadableStream } from "http://localhost:8080/react-dom@18/server";

Deno.test("react-18-stream", async () => {
  const res = new Response(
    await renderToReadableStream(
      <main>
        <h1>Hi :)</h1>
      </main>,
    ),
  );
  const html = await res.text();
  assertStringIncludes(html, "<main");
  assertStringIncludes(html, "<h1>Hi :)</h1>");
  assertStringIncludes(html, "</main>");
});

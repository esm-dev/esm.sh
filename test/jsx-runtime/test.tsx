import { assert, assertStringIncludes } from "jsr:@std/assert";

import { Airplay } from "http://localhost:8080/gh/phosphor-icons/react@v2.1.5/src/csr/Airplay.tsx?deps=react@18.2.0";
import { renderToString } from "http://localhost:8080/react-dom@18.2.0/server";

Deno.test("jsx-runtime", async () => {
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

Deno.test("react-jsx-runtime deps", async () => {
  const res = await fetch("http://localhost:8080/react@18.2.0/esnext/jsx-runtime.js");
  assertStringIncludes(await res.text(), `from "/react@18.2.0/esnext/react.mjs"`);
});

Deno.test("rendering a svg from github.com", async () => {
  const svg = renderToString(<Airplay />);
  assertStringIncludes(svg, "<svg ");
  assertStringIncludes(svg, "</svg>");
});

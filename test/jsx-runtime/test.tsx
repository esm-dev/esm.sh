import { assertStringIncludes } from "jsr:@std/assert";

import { Airplay } from "http://localhost:8080/gh/phosphor-icons/react@v2.1.5/src/csr/Airplay.tsx?deps=react@18.2.0";
import { renderToString } from "http://localhost:8080/react-dom@18.2.0/server";

Deno.test("jsx-runtime", async () => {
  const svg = renderToString(<Airplay />);
  assertStringIncludes(svg, "<svg ");
  assertStringIncludes(svg, "</svg>");
});

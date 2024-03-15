import { assertStringIncludes } from "https://deno.land/std@0.220.0/assert/mod.ts";

Deno.test("support exports `development` condition", async () => {
  const code = await fetch("http://localhost:8080/react@18.2.0/es2022/react.development.mjs").then((
    res,
  ) => res.text());
  assertStringIncludes(code, "/cjs/react.development.js");

  const code2 = await fetch("http://localhost:8080/solid-js@1.6.16/es2022/solid-js.development.mjs").then((
    res,
  ) => res.text());
  assertStringIncludes(code2, "/dist/dev.js");
});

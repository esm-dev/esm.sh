import {
  assertStringIncludes,
} from "https://deno.land/std@0.180.0/testing/asserts.ts";

Deno.test("support exports `development` condition", async () => {
  const code = await fetch("http://localhost:8080/stable/react@18.2.0/es2022/react.development.mjs").then((
    res,
  ) => res.text());
  assertStringIncludes(code, "/cjs/react.development.js");

  const code2 = await fetch("http://localhost:8080/stable/solid-js@1.6.16/es2022/solid-js.development.mjs").then((
    res,
  ) => res.text());
  assertStringIncludes(code2, "/dist/dev.js");
});

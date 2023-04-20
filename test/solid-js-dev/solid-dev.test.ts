import {
  assertStringIncludes,
} from "https://deno.land/std@0.180.0/testing/asserts.ts";

Deno.test("solid.js?dev ssr ", async () => {
  const code = await fetch("http://localhost:8080/stable/solid-js@1.6.16/es2022/solid-js.development.mjs").then((
    res,
  ) => res.text());
  assertStringIncludes(code, "/dist/dev.js");
});

import { assertStringIncludes } from "jsr:@std/assert";

Deno.test("?dev", async () => {
  const res = await fetch("http://localhost:8080/react@18.2.0?dev&target=es2022");
  assertStringIncludes(await res.text(), `"/react@18.2.0/es2022/react.development.mjs"`);

  const code = await fetch("http://localhost:8080/react@18.2.0/es2022/react.development.mjs").then((res) => res.text());
  assertStringIncludes(code, "react/cjs/react.development.js");
});

Deno.test("using `development` condition in `exports`", async () => {
  const code2 = await fetch("http://localhost:8080/solid-js@1.6.16/es2022/solid-js.development.mjs").then((res) => res.text());
  assertStringIncludes(
    code2,
    `console.warn("You appear to have multiple instances of Solid. This can lead to unexpected behavior.")`,
  );
});

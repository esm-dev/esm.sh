import {
  assertEquals,
  assertStringIncludes,
} from "https://deno.land/std@0.180.0/testing/asserts.ts";

Deno.test("`?deno-std` query", async () => {
  const code = await fetch(
    `http://localhost:8080/postcss@8.4.14?target=deno&deno-std=0.128.0`,
  ).then((res) => res.text());
  const [, v, d] = code.match(/\/(v\d+)\/postcss@8.4.14\/(.+)\/postcss.mjs"/)!;
  assertEquals(d, "X-ZHN2LzAuMTI4LjA/deno");
  assertStringIncludes(
    await fetch(`http://localhost:8080/${v}/postcss@8.4.14/${d}/lib/postcss.js`)
      .then((res) => res.text()),
    "https://deno.land/std@0.128.0/",
  );
});

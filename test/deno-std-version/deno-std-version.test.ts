import {
  assertStringIncludes,
} from "https://deno.land/std@0.155.0/testing/asserts.ts";

Deno.test("`?deno-std` query", async () => {
  const entryCode = await fetch(
    `http://localhost:8080/postcss@8.4.14?deno-std=0.128.0`,
  ).then((res) => res.text());
  const url = new URL(entryCode.split('"')[1]);
  assertStringIncludes(
    url.pathname,
    "/X-ZHN2LzAuMTI4LjA/deno",
  );
  assertStringIncludes(
    await fetch(url).then((res) => res.text()),
    "https://deno.land/std@0.128.0/",
  );
});

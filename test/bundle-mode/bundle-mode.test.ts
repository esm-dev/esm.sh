import {
  assertStringIncludes,
} from "https://deno.land/std@0.180.0/testing/asserts.ts";

Deno.test("bundle mode ", async () => {
  const code = await fetch(
    "http://localhost:8080/v126/buffer@6.0.3/es2022/buffer.bundle.mjs",
  ).then((res) => res.text());
  assertStringIncludes(code, `"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"`);
});

import { assertExists } from "https://deno.land/std@0.180.0/testing/asserts.ts";

Deno.test("Nested conditions", async () => {
  const { version } = await fetch("http://localhost:8080/status.json").then((
    res,
  ) => res.json());
  const utils = await import(
    `http://localhost:8080/v${version}/jotai@2.0.3/es2022/vanilla/utils.js`
  );
  assertExists(utils.splitAtom);
});

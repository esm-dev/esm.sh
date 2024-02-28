import { assertExists } from "https://deno.land/std@0.210.0/testing/asserts.ts";

Deno.test("Nested conditions", async () => {
  const utils = await import(
    `http://localhost:8080/jotai@2.0.3/es2022/vanilla/utils.js`
  );
  assertExists(utils.splitAtom);
});

import { assertExists } from "jsr:@std/assert";

Deno.test("jotail", async () => {
  const utils = await import("http://localhost:8080/jotai@2.0.3/es2022/vanilla/utils.mjs");
  assertExists(utils.splitAtom);
});

import { assertExists } from "jsr:@std/assert";

Deno.test("Nested conditions", async () => {
  const utils = await import("http://localhost:8080/jotai@2.0.3/es2022/vanilla/utils.js");
  assertExists(utils.splitAtom);
});

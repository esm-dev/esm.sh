import { assertExists } from "https://deno.land/std@0.178.0/testing/asserts.ts";

import * as utils from 'http://localhost:8080/v111/jotai@2.0.3/es2022/vanilla/utils.js'

Deno.test("Nested conditions", async () => {
  assertExists(utils.splitAtom);
});

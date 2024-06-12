import { assertExists } from "https://deno.land/std@0.220.0/assert/mod.ts";

import * as tslib from "http://localhost:8080/gh/microsoft/tslib";

Deno.test("github module", async () => {
  assertExists(tslib.__await);
});

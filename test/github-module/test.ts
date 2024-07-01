import { assertExists } from "jsr:@std/assert";

import * as tslib from "http://localhost:8080/gh/microsoft/tslib";

Deno.test("github module", async () => {
  assertExists(tslib.__await);
});

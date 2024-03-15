import { assertExists } from "https://deno.land/std@0.220.0/assert/mod.ts";

import * as dynamicCSS from "http://localhost:8080/rc-util@5.27.2/es/Dom/dynamicCSS.js";

Deno.test("Export all members when the package is not a standard ES module", async () => {
  assertExists(dynamicCSS.updateCSS);
  assertExists(dynamicCSS.injectCSS);
  assertExists(dynamicCSS.removeCSS);
  assertExists(dynamicCSS.clearContainerCache);
});

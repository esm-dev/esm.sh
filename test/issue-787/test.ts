import { assertExists } from "jsr:@std/assert";

import * as capabilities from "http://localhost:8080/@web3-storage/capabilities@^18.0.0/index?target=denonext";

Deno.test("issue #787", () => {
  assertExists(capabilities.add);
  assertExists(capabilities.add.can);
});

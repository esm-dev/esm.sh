import { assert } from "jsr:@std/assert";

import "http://localhost:8080/ajv@8.12.0?bundle&external=fast-deep-equal";

Deno.test("external-bundle", async () => {
  assert((globalThis as any).ourDeepEqImported === true);
});

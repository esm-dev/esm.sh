import { assert } from "https://deno.land/std@0.170.0/testing/asserts.ts";

import * as ajv from 'http://localhost:8080/ajv@8.12.0?bundle&external=fast-deep-equal'

function use(arg: any) {

}

Deno.test("external-bundle", async () => {
  use(ajv)
  assert((globalThis as any).ourDeepEqImported === true)
  // assert(html == "<main><p>just now</p></main>");
});

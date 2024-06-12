import { assertArrayIncludes } from "https://deno.land/std@0.220.0/assert/mod.ts";

import * as mod from "http://localhost:8080/react-svg-spinners@0.3.1";

Deno.test("issue #512", () => {
  assertArrayIncludes(Object.keys(mod), [
    "NinetyRing",
    "NinetyRingWithBg",
    "default",
  ]);
});

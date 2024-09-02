import { assertArrayIncludes } from "jsr:@std/assert";

import * as mod from "http://localhost:8080/react-svg-spinners@0.3.1";

Deno.test("issue #512", () => {
  assertArrayIncludes(Object.keys(mod), [
    "NinetyRing",
    "NinetyRingWithBg",
    "default",
  ]);
});

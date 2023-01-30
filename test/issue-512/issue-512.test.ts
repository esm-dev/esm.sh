import { assertEquals } from "https://deno.land/std@0.170.0/testing/asserts.ts";

import * as mod from "http://localhost:8080/react-svg-spinners@0.3.1?cjs-exports=NinetyRing,NinetyRingWithBg";

Deno.test("issue #512", () => {
  assertEquals(Object.keys(mod).sort(), [
    "NinetyRing",
    "NinetyRingWithBg",
    "default",
  ]);
});

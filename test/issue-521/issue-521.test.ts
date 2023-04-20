import { assertEquals } from "https://deno.land/std@0.180.0/testing/asserts.ts";

import * as cm from "http://localhost:8080/codemirror@6.0.1?exports=minimalSetup"

Deno.test("issue #521", () => {
  assertEquals(Object.keys(cm), ["minimalSetup"]);
});

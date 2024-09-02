import { assertEquals } from "jsr:@std/assert";

import * as cm from "http://localhost:8080/codemirror@6.0.1?exports=minimalSetup";

Deno.test("issue #521", () => {
  assertEquals(Object.keys(cm), ["minimalSetup"]);
});

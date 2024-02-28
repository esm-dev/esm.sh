import { assertEquals } from "https://deno.land/std@0.210.0/testing/asserts.ts";

import jsGrammar from "http://localhost:8080/@wooorm/starry-night@2.0.0/lang/source.js.js";
import tsGrammar from "http://localhost:8080/@wooorm/starry-night@2.0.0/lang/source.ts.js";

Deno.test("issue #627", async () => {
  assertEquals(jsGrammar.scopeName, "source.js");
  assertEquals(tsGrammar.scopeName, "source.ts");
});

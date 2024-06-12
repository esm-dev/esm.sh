import { assertEquals } from "https://deno.land/std@0.220.0/assert/mod.ts";

import { load } from "http://localhost:8080/cheerio@0.22.0";
import SymbolPolyfill from "http://localhost:8080/es6-symbol@3.1.3";

Deno.test("issue #629", async () => {
  assertEquals(typeof load, "function");
  assertEquals(typeof SymbolPolyfill, "function");
  assertEquals(SymbolPolyfill, Symbol);
});

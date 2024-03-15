import { assertEquals } from "https://deno.land/std@0.220.0/assert/mod.ts";

import HTTP from "http://localhost:8080/ipfs-utils@9.0.14/src/http.js";

Deno.test("issue #553", () => {
  assertEquals(typeof HTTP.get, "function");
});

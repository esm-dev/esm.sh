import { assertEquals } from "https://deno.land/std@0.180.0/testing/asserts.ts";

import HTTP from "http://localhost:8080/ipfs-utils@9.0.14/src/http.js"

Deno.test("issue #553", () => {
  assertEquals(typeof HTTP.get, "function");
});

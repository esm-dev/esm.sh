import { assertEquals } from "jsr:@std/assert";

import HTTP from "http://localhost:8080/ipfs-utils@9.0.14/src/http.js?target=es2022";

Deno.test("issue #553", () => {
  assertEquals(typeof HTTP.get, "function");
});

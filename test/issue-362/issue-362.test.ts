import { assertEquals } from "https://deno.land/std@0.170.0/testing/asserts.ts";

import { codes } from "http://localhost:8080/keycode@2.2.1";
import { loadWASM } from "http://localhost:8080/vscode-oniguruma@1.6.2";

Deno.test("issue #362", () => {
  assertEquals(typeof codes, "object");
  assertEquals(typeof loadWASM, "function");
});

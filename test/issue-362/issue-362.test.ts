import { equal } from "https://deno.land/std@0.145.0/testing/asserts.ts";

import { codes } from "http://localhost:8080/keycode@2.2.1";
import { loadWASM } from "http://localhost:8080/vscode-oniguruma@1.6.2";

Deno.test("issue #362", () => {
  equal(typeof codes, "object");
  equal(typeof loadWASM, "function");
});

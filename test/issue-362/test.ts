import { assertEquals } from "jsr:@std/assert";

import { codes } from "http://localhost:8080/keycode@2.2.1";
import { loadWASM } from "http://localhost:8080/vscode-oniguruma@1.6.2";

Deno.test("issue #362", () => {
  assertEquals(typeof codes, "object");
  assertEquals(typeof loadWASM, "function");
});

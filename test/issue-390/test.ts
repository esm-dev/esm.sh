import { assertEquals } from "jsr:@std/assert";

import { getHighlighter } from "http://localhost:8080/shiki@0.10.1";
import {
  INITIAL,
  parseRawGrammar,
  Registry,
} from "http://localhost:8080/vscode-textmate@5.2.0";

Deno.test("issue #390 (shiki CommonJS exports)", () => {
  assertEquals(typeof getHighlighter, "function");
  assertEquals(INITIAL.depth, 1);
  assertEquals(typeof Registry, "function");
  assertEquals(
    parseRawGrammar('{"scopeName":"source.test","patterns":[]}', "test.json")
      .scopeName,
    "source.test",
  );
});

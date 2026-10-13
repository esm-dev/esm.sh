import { assertEquals, assertStrictEquals } from "jsr:@std/assert";

import * as Blockly from "http://localhost:8080/blockly@11.1.1";
import * as core from "http://localhost:8080/blockly@11.1.1/core";
import { blocks } from "http://localhost:8080/blockly@11.1.1/blocks";
import { javascriptGenerator } from "http://localhost:8080/blockly@11.1.1/javascript";

Deno.test("issue-878", () => {
  assertEquals(Blockly.VERSION, "11.1.1");
  assertStrictEquals(Blockly.Blocks, core.Blocks);
  assertStrictEquals(Blockly.Blocks["math_number"], blocks["math_number"]);
  Blockly.Events.disable();
  const workspace = new Blockly.Workspace();
  try {
    const block = workspace.newBlock("math_number");
    block.setFieldValue(42, "NUM");
    assertEquals(javascriptGenerator.workspaceToCode(workspace).trim(), "42;");
  } finally {
    workspace.dispose();
    Blockly.Events.enable();
  }
});

import { assertEquals } from "jsr:@std/assert";

import * as ts from "http://localhost:8080/typescript@5.5.4";

Deno.test("typescript", () => {
  const result = ts.transpileModule(`let x: string  = "string"`, {
    compilerOptions: { module: ts.ModuleKind.CommonJS },
  });
  assertEquals(ts.version, "5.5.4");
  assertEquals(result.outputText, `var x = "string";\n`);
});

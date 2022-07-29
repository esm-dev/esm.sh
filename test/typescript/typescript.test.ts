import { assertEquals } from "https://deno.land/std@0.145.0/testing/asserts.ts";

import * as ts from "http://localhost:8080/typescript@4.6.2";

Deno.test("typescript", async () => {
  const result = ts.transpileModule(`let x: string  = "string"`, {
    compilerOptions: { module: ts.ModuleKind.CommonJS },
  });
  assertEquals(ts.version, "4.6.2");
  assertEquals(result.outputText, `var x = "string";\n`);
});

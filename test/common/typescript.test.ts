import { assert, assertEquals, assertStringIncludes } from "jsr:@std/assert";

import * as ts from "http://localhost:8080/typescript@5.5.4";
import * as ts2 from "http://localhost:8080/typescript@5.5.4?target=es2022";

Deno.test("typescript", async () => {
  const result = ts.transpileModule(`let x: string  = "string"`, {
    compilerOptions: { module: ts.ModuleKind.CommonJS },
  });
  assertEquals(ts.version, "5.5.4");
  assertEquals(result.outputText, `var x = "string";\n`);
});

Deno.test("typescript (target=es2022)", async () => {
  const result = ts2.transpileModule(`let x: string  = "string"`, {
    compilerOptions: { module: ts2.ModuleKind.CommonJS },
  });
  assertEquals(ts2.version, "5.5.4");
  assertEquals(result.outputText, `var x = "string";\n`);
});

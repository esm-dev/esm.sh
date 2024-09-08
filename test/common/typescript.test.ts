import { assert, assertEquals, assertStringIncludes } from "jsr:@std/assert";

import * as ts from "http://localhost:8080/typescript@5.5.4";
import * as ts2 from "http://localhost:8080/typescript@5.5.4?target=esnext";

Deno.test("typescript", async () => {
  const result = ts.transpileModule(`let x: string  = "string"`, {
    compilerOptions: { module: ts.ModuleKind.CommonJS },
  });
  assertEquals(ts.version, "5.5.4");
  assertEquals(result.outputText, `var x = "string";\n`);

  const res = await fetch("http://localhost:8080/typescript@5.5.4/denonext/typescript.mjs");
  const js = await res.text();
  assertStringIncludes(js, 'import __Process$ from "node:process"');
});

Deno.test("typescript (target=esnext)", async () => {
  const result = ts2.transpileModule(`let x: string  = "string"`, {
    compilerOptions: { module: ts2.ModuleKind.CommonJS },
  });
  assertEquals(ts2.version, "5.5.4");
  assertEquals(result.outputText, `var x = "string";\n`);

  const res = await fetch("http://localhost:8080/typescript@5.5.4/esnext/typescript.mjs");
  const js = await res.text();
  assertStringIncludes(js, "const __Process$ = {env:{}};");
});

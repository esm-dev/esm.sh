import { assertEquals, assertStringIncludes } from "jsr:@std/assert";
import { Buffer } from "node:buffer";

import init from "http://localhost:8080/esm-compiler@0.5.2";
import { transform } from "http://localhost:8080/esm-compiler@0.5.2/swc";
import { transform as transformCSS } from "http://localhost:8080/esm-compiler@0.5.2/lightningcss";

Deno.test("esm-compiler", async () => {
  await init("http://localhost:8080/esm-compiler@0.5.2/pkg/esm_compiler_bg.wasm");
  const ret = transform("index.ts", "const n:number = 123')", {});
  assertEquals(ret.code, "const n = 123;\n");
  const ret2 = transformCSS({ filename: "index.module.css", code: Buffer.from(".foo{color:red}"), minify: true });
  assertStringIncludes(ret2.code, "_foo{color:red}");
  assertEquals([...ret2.exports!.keys()], ["foo"]);
});

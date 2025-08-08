import { assertEquals } from "jsr:@std/assert";

Deno.test("typescript", async () => {
  const ts = await import("http://localhost:8080/typescript@5.5.4");
  const result = ts.transpileModule(`let x: string = "string"`, {
    compilerOptions: { module: ts.ModuleKind.CommonJS },
  });
  assertEquals(ts.version, "5.5.4");
  assertEquals(result.outputText, `var x = "string";\n`);
});

Deno.test("typescript (target=browser)", async () => {
  const js = await fetch("http://localhost:8080/typescript@5.5.4", {
    headers: {
      "user-agent": "i'm a browser",
    },
  }).then(res => res.text());
  if (/\/node\/\w+\.mjs/.test(js)) {
    throw new Error("node builtin modules should not be included in browser target");
  }
});

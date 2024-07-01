import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("issue #640", async () => {
  const res = await fetch(
    `http://localhost:8080/lightningcss-wasm@1.20.0/index.d.ts`,
    { redirect: "manual" },
  );
  assertEquals(res.status, 200);
  assertEquals(
    res.headers.get("content-type")!,
    "application/typescript; charset=utf-8",
  );
  assertStringIncludes(
    await res.text(),
    ["export interface ImportDependency {", "  type: 'import',"].join("\n"),
  );
});

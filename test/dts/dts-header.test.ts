import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("dts header", async () => {
  const res = await fetch("http://localhost:8080/@jridgewell/trace-mapping@0.3.31");
  res.body?.cancel();
  assertEquals(res.status, 200);
  assertEquals(res.headers.get("content-type"), "application/javascript; charset=utf-8");
  assertEquals(res.headers.get("x-typescript-types"), "http://localhost:8080/@jridgewell/trace-mapping@0.3.31/types/trace-mapping.d.mts");
  const dts = await fetch(res.headers.get("x-typescript-types")!).then((r) => r.text());
  assertStringIncludes(dts, `'./sourcemap-segment.d.mts'`);

  for (const i of [1, 2, 3]) { // repeat 3 times
    const res = await fetch("http://localhost:8080/preact@10.28.4/es2022/preact.mjs");
    res.body?.cancel();
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("content-type"), "application/javascript; charset=utf-8");
    assertEquals(res.headers.get("x-typescript-types"), "http://localhost:8080/preact@10.28.4/src/index.d.ts");
    const dts = await fetch(res.headers.get("x-typescript-types")!).then((r) => r.text());
    assertStringIncludes(dts, `'./jsx.d.ts'`);
  }
});

import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("dts header", async () => {
  const res = await fetch("http://localhost:8080/@jridgewell/trace-mapping@0.3.31?origin-regression", {
    headers: {
      "X-Real-Origin": `https://attacker.invalid" } = options; globalThis.PWNED = true; const { z = "`,
    },
  });
  res.body?.cancel();
  assertEquals(res.status, 200);
  assertEquals(res.headers.get("content-type"), "application/javascript; charset=utf-8");
  assertEquals(res.headers.get("x-typescript-types"), "/@jridgewell/trace-mapping@0.3.31/types/trace-mapping.d.mts");
  const dts = await fetch(new URL(res.headers.get("x-typescript-types")!, res.url)).then((r) => r.text());
  assertStringIncludes(dts, `'./sourcemap-segment.d.mts'`);
});

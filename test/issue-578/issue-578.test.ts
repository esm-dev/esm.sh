import { assertEquals } from "https://deno.land/std@0.180.0/testing/asserts.ts";

Deno.test("issue #578", async () => {
  const res = await fetch(`http://localhost:8080/katex@0.16.4/dist/katex.mjs`);
  const tsHeader = res.headers.get("x-typescript-types");
  res.body?.cancel();
  assertEquals(
    tsHeader,
    `http://localhost:8080/@types/katex@~0.16/index.d.ts`,
  );
});

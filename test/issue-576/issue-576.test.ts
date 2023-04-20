import { assertEquals } from "https://deno.land/std@0.180.0/testing/asserts.ts";

Deno.test("issue #576", async () => {
  const res = await fetch("http://localhost:8080/v113/dedent@0.7.0");
  const tsHeader = res.headers.get("x-typescript-types");
  res.body?.cancel();
  assertEquals(
    tsHeader,
    "http://localhost:8080/v113/@types/dedent@~0.7/index.d.ts"
  );
});

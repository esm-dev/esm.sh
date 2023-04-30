import { assertEquals } from "https://deno.land/std@0.180.0/testing/asserts.ts";

Deno.test("issue #576", async () => {
  const { version } = await fetch("http://localhost:8080/status.json").then((
    res,
  ) => res.json());
  const res = await fetch(`http://localhost:8080/v${version}/dedent@0.7.0`);
  const tsHeader = res.headers.get("x-typescript-types");
  res.body?.cancel();
  assertEquals(
    tsHeader,
    `http://localhost:8080/v${version}/@types/dedent@~0.7/index.d.ts`
  );
});

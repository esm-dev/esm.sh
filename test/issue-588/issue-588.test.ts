import { assertStringIncludes } from "https://deno.land/std@0.210.0/testing/asserts.ts";

Deno.test("issue #588", async () => {
  const code = await fetch(
    `http://localhost:8080/@superfluid-finance/sdk-core@0.6.3/es2020/sdk-core.mjs`,
  ).then((res) => res.text());

  assertStringIncludes(
    code,
    `"/gh/superfluid-finance/metadata"`,
  );
});

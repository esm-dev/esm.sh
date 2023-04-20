import { assertStringIncludes } from "https://deno.land/std@0.180.0/testing/asserts.ts";

Deno.test("issue #588", async () => {
  const { version } = await fetch("http://localhost:8080/status.json").then((
    res,
  ) => res.json());
  const code = await fetch(
    `http://localhost:8080/v${version}/@superfluid-finance/sdk-core@0.6.3/es2020/sdk-core.mjs`,
  ).then((res) => res.text());

  assertStringIncludes(
    code,
    `"/v${version}/gh/superfluid-finance/metadata"`,
  );
});

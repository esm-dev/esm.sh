import { assertStringIncludes } from "https://deno.land/std@0.220.0/assert/mod.ts";

Deno.test("issue #588", async () => {
  const res = await fetch("http://localhost:8080/@superfluid-finance/sdk-core@0.6.3/es2020/sdk-core.mjs");
  const code = await res.text();
  assertStringIncludes(code, `"/gh/superfluid-finance/metadata@`);
});

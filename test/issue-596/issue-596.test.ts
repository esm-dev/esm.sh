import { assert } from "https://deno.land/std@0.180.0/testing/asserts.ts";

Deno.test("issue #596", async () => {
  const { version } = await fetch("http://localhost:8080/status.json").then((
    res,
  ) => res.json());
  const code = await fetch(
    `http://localhost:8080/v${version}/reejs@0.9.0/deno/src/cli/index.js`,
  ).then((res) => res.text());
  assert(!code.includes("#!/usr/bin/env node"));
});

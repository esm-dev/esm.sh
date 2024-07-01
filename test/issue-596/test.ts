import { assert } from "jsr:@std/assert";

Deno.test("issue #596", async () => {
  const code = await fetch(
    `http://localhost:8080/reejs@0.9.0/deno/src/cli/index.js`,
  ).then((res) => res.text());
  assert(!code.includes("#!/usr/bin/env node"));
});

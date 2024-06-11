import { assert, assertEquals, assertStringIncludes } from "https://deno.land/std@0.220.0/assert/mod.ts";

import * as t from "http://localhost:8080/io-ts@2.2.21";
import { isRight } from "http://localhost:8080/fp-ts@2.16.6/lib/Either";

const string = new t.Type<string, string, unknown>(
  "string",
  (input: unknown): input is string => typeof input === "string",
  // `t.success` and `t.failure` are helpers used to build `Either` instances
  (
    input,
    context,
  ) => (typeof input === "string" ? t.success(input) : t.failure(input, context)),
  // `A` and `O` are the same, so `encode` is just the identity function
  t.identity,
);

Deno.test("io-ts", async () => {
  assert(isRight(string.decode("a string")));
  assert(!isRight(string.decode(null)));
  const res = await fetch("http://localhost:8080/fp-ts@2.16.6/lib/Extend.d.ts");
  assertEquals(res.status, 200);
  assertEquals(res.headers.get("Content-Type"), "application/typescript; charset=utf-8");
  assertStringIncludes(await res.text(), "'../HKT.d.ts'");
});

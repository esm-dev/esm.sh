import { assert } from "https://deno.land/std@0.210.0/testing/asserts.ts";

import * as t from "http://localhost:8080/io-ts";
import { isRight } from "http://localhost:8080/fp-ts/lib/Either";

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
});

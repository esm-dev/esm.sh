import { assertExists } from "https://deno.land/std@0.178.0/testing/asserts.ts";

import * as graphql from 'http://localhost:8080/graphql@16.6.0'

Deno.test("Correct entry point", async () => {
  assertExists(graphql.GraphQLNonNull);
});

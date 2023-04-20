import { assertEquals } from "https://deno.land/std@0.180.0/testing/asserts.ts";

import { filterCookies } from "http://localhost:8080/@supabase/auth-helpers-shared@0.3.0";

Deno.test("issue #572", async () => {
  assertEquals(filterCookies(["foo=", "bar="], "foo"), ["bar="]);
});

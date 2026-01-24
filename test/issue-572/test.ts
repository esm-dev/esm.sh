import { assertEquals } from "jsr:@std/assert";

import { filterCookies } from "http://localhost:8080/@supabase/auth-helpers-shared@0.3.0?deps=@supabase/supabase-js@2.91.0";

Deno.test("issue #572", async () => {
  assertEquals(filterCookies(["foo=", "bar="], "foo"), ["bar="]);
});

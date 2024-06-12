import { assert } from "https://deno.land/std@0.220.0/assert/mod.ts";

import urlRegexSafe from "http://localhost:8080/url-regex-safe@3.0.0";

Deno.test("issue #591", async () => {
  assert(urlRegexSafe() instanceof RegExp);
});

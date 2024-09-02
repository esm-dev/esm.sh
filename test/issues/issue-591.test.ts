import { assert } from "jsr:@std/assert";

import urlRegexSafe from "http://localhost:8080/url-regex-safe@3.0.0";

Deno.test("issue #591", async () => {
  assert(urlRegexSafe() instanceof RegExp);
});

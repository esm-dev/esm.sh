import { assertEquals } from "https://deno.land/std@0.220.0/assert/mod.ts";

import { unzip } from "http://localhost:8080/@gmod/bgzf-filehandle@1.4.5";

Deno.test("issue #628", async () => {
  assertEquals(typeof unzip, "function");
});

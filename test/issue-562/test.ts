import { assertEquals } from "https://deno.land/std@0.220.0/assert/mod.ts";

import { upnpNat } from "http://localhost:8080/@achingbrain/nat-port-mapper@1.0.7";

Deno.test("issue #562", () => {
  assertEquals(typeof upnpNat, "function");
});

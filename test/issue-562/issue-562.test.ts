import { assertEquals } from "https://deno.land/std@0.178.0/testing/asserts.ts";

import { upnpNat } from 'http://localhost:8080/@achingbrain/nat-port-mapper@1.0.7'

Deno.test("issue #562", () => {
  assertEquals(typeof upnpNat, "function");
});

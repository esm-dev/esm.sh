import { assertEquals } from "jsr:@std/assert";

import { upnpNat } from "http://localhost:8080/@achingbrain/nat-port-mapper@1.0.7";

Deno.test("issue #562", () => {
  assertEquals(typeof upnpNat, "function");
});

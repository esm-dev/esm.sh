import { assertEquals } from "https://deno.land/std@0.220.0/assert/mod.ts";

import { BigInteger } from "http://localhost:8080/jsbn@1.1.0";
import { Netmask } from "http://localhost:8080/netmask@2.0.2";
import { parseStringPromise } from "http://localhost:8080/xml2js@0.4.23";
import compareVersions from "http://localhost:8080/tiny-version-compare@3.0.1";

Deno.test("misc", () => {
  assertEquals(typeof BigInteger, "function");
  assertEquals(typeof Netmask, "function");
  assertEquals(typeof parseStringPromise, "function");
  assertEquals(compareVersions("1.12.0", "v1.12.0"), 0);
});

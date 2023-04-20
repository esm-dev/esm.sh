import { assertEquals } from "https://deno.land/std@0.180.0/testing/asserts.ts";

import { BigInteger } from "http://localhost:8080/jsbn@1.1.0";
import { Netmask } from "http://localhost:8080/netmask@2.0.2";
import { parseStringPromise } from "http://localhost:8080/xml2js@0.4.23";

Deno.test("ipfs-dependencies", () => {
  assertEquals(typeof BigInteger, "function");
  assertEquals(typeof Netmask, "function");
  assertEquals(typeof parseStringPromise, "function");
});

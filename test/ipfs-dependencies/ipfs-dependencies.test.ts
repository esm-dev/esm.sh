import { assertEquals } from "https://deno.land/std@0.178.0/testing/asserts.ts";

import { BigInteger } from "http://localhost:8080/jsbn@1.1.0";

Deno.test("ipfs-dependency-jsbn", () => {
  assertEquals(typeof BigInteger, "function");
});

import { Netmask } from "http://localhost:8080/netmask@2.0.2";

Deno.test("ipfs-dependency-netmask", () => {
  assertEquals(typeof Netmask, "function");
});

import { parseStringPromise } from "http://localhost:8080/xml2js@0.4.23";

Deno.test("ipfs-dependency-xml2js", () => {
  assertEquals(typeof parseStringPromise, "function");
});

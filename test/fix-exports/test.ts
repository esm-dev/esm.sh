import { assertEquals } from "jsr:@std/assert";

import { BigInteger } from "http://localhost:8080/jsbn@1.1.0";
import { Netmask } from "http://localhost:8080/netmask@2.0.2";
import { parseStringPromise } from "http://localhost:8080/xml2js@0.4.23";
import compareVersions from "http://localhost:8080/tiny-version-compare@3.0.1";
import { createTheme } from "http://localhost:8080/baseui@12.2.0";

Deno.test("fix some invalid exports", () => {
  assertEquals(typeof BigInteger, "function");
  assertEquals(typeof Netmask, "function");
  assertEquals(typeof parseStringPromise, "function");
  assertEquals(typeof createTheme, "function");
  assertEquals(typeof compareVersions, "function");
});

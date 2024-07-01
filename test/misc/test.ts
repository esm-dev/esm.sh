import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

import { BigInteger } from "http://localhost:8080/jsbn@1.1.0";
import { Netmask } from "http://localhost:8080/netmask@2.0.2";
import { parseStringPromise } from "http://localhost:8080/xml2js@0.4.23";
import hljs from "http://localhost:8080/highlight.js@11.9.0/lib/core";
import type { HLJSApi } from "http://localhost:8080/highlight.js@11.9.0";
import compareVersions from "http://localhost:8080/tiny-version-compare@3.0.1";
import { createTheme } from "http://localhost:8080/baseui@12.2.0";

Deno.test("misc", () => {
  assertEquals(typeof BigInteger, "function");
  assertEquals(typeof Netmask, "function");
  assertEquals(typeof parseStringPromise, "function");
  assertEquals(typeof (hljs satisfies HLJSApi), "object");
  assertEquals(typeof hljs.highlight, "function");
  assertEquals(typeof createTheme, "function");
  assertEquals(compareVersions("1.12.0", "v1.12.0"), 0);
});

Deno.test("dts-transformer: support `.d` extension", async () => {
  const res = await fetch("http://localhost:8080/tailwindcss@3.3.5/types/index.d.ts");
  const dts = await res.text();
  assertStringIncludes(dts, "'./config.d.ts'");
});

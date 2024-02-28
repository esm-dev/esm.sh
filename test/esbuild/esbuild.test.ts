import { assertEquals } from "https://deno.land/std@0.180.0/testing/asserts.ts";

import esbuild, { version } from "http://localhost:8080/esbuild@0.17.18";

Deno.test(
  "esbuild",
  { sanitizeOps: false, sanitizeResources: false },
  () => {
    assertEquals(version, "0.17.18");
    assertEquals(esbuild.version, "0.17.18");
  },
);

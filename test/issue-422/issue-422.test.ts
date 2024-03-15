import { assertEquals } from "https://deno.land/std@0.220.0/assert/mod.ts";

import * as nfn from "http://localhost:8080/node-fetch-native";

Deno.test("issue #422", () => {
  // @ts-ignore
  assertEquals(nfn.fileFrom, undefined);
});

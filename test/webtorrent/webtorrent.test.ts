import { assertEquals } from "https://deno.land/std@0.220.0/assert/mod.ts";

import webtorrent from "http://localhost:8080/webtorrent@2.0.18?target=es2022&no-dts";

Deno.test("webtorrent", async () => {
  assertEquals(typeof webtorrent, "function");
});

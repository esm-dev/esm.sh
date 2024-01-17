import {
  assertEquals,
  assertStringIncludes,
} from "https://deno.land/std@0.180.0/testing/asserts.ts";

Deno.test("raw untransformed JS via ?raw query", async () => {
  const res = await fetch(
    "http://localhost:8080/playground-elements@0.18.1/playground-service-worker.js?raw",
  );
  assertEquals(res.status, 200);
  assertEquals(
    res.headers.get("content-type"),
    "application/javascript; charset=utf-8",
  );
  assertStringIncludes(await res.text(), "!function(){");
});

Deno.test("raw untransformed JS via &raw extra query", async () => {
  const res = await fetch(
    "http://localhost:8080/playground-elements@0.18.1&raw/playground-service-worker.js",
  );
  assertEquals(res.status, 200);
  assertEquals(
    res.headers.get("content-type"),
    "application/javascript; charset=utf-8",
  );
  assertStringIncludes(await res.text(), "!function(){");
});

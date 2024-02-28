import { assertEquals } from "https://deno.land/std@0.210.0/testing/asserts.ts";

import { Map } from "http://localhost:8080/maplibre-gl@1.15.3";

Deno.test("issue #454", () => {
  assertEquals(typeof Map, "function");
});

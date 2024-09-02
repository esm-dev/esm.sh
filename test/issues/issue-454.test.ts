import { assertEquals } from "jsr:@std/assert";

import { Map } from "http://localhost:8080/maplibre-gl@1.15.3";

Deno.test("issue #454", () => {
  assertEquals(typeof Map, "function");
});

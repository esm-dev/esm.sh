import { assert } from "https://deno.land/std@0.220.0/assert/mod.ts";

import geojsonRbush from "http://localhost:8080/geojson-rbush@3.2.0";

Deno.test("issue #620", async () => {
  const tree = geojsonRbush();
  assert(Array.isArray(tree.toJSON().children));
});

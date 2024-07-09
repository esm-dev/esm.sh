import { assertEquals } from "jsr:@std/assert";
import * as prod from "esm.sh/react@19.0.0-rc-58af67a8f8-20240628";
import * as dev from "esm.sh/react@19.0.0-rc-58af67a8f8-20240628?dev";

// related issue: https://github.com/esm-dev/esm.sh/issues/847
Deno.test("issue #847", async () => {
  assertEquals(Object.keys(prod), Object.keys(dev));
});

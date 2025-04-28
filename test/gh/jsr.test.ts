import { assertEquals } from "jsr:@std/assert";

import $ from "http://localhost:8080/gh/dsherret/dax@6aed9b0";

Deno.test("jsr package from github", async () => {
  assertEquals(typeof $, "function");
  assertEquals(await $`echo 42`.text(), "42");
});

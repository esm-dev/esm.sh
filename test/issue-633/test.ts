import { assertEquals } from "jsr:@std/assert";

import { map } from "http://localhost:8080/rxjs@7.8.1/operators";

Deno.test("issue #633", async () => {
  assertEquals(typeof map, "function");
});

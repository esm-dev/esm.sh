import { assertEquals } from "jsr:@std/assert";

import { onMount } from "http://localhost:8080/svelte@5.16.0?target=es2022";

Deno.test("svelte runtime", async () => {
  assertEquals(typeof onMount, "function");
});

import { assertEquals } from "https://deno.land/std@0.220.0/assert/mod.ts";

import { fs } from "http://localhost:8080/memfs";

Deno.test("issue #363", () => {
  fs.writeFileSync("/hello.txt", "World!");
  assertEquals(fs.readFileSync("/hello.txt", "utf8"), "World!");
});

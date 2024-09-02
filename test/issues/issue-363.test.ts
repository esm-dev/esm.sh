import { assertEquals } from "jsr:@std/assert";

import { fs } from "http://localhost:8080/memfs";

Deno.test("issue #363", () => {
  fs.writeFileSync("/hello.txt", "World!");
  assertEquals(fs.readFileSync("/hello.txt", "utf8"), "World!");
});

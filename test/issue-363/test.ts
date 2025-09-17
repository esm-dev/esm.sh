import { assertEquals } from "jsr:@std/assert";

import { fs } from "http://localhost:8080/memfs@4.39.0";

Deno.test("issue #363", () => {
  fs.writeFileSync("/hello.txt", "World!");
  assertEquals(fs.readFileSync("/hello.txt", "utf8"), "World!");
});

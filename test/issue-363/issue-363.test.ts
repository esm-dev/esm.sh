import { equal } from "https://deno.land/std@0.145.0/testing/asserts.ts";

import { fs } from "http://localhost:8080/memfs";

Deno.test("issue #363", () => {
  fs.writeFileSync("/hello.txt", "World!");
  equal(fs.readFileSync("/hello.txt", "utf8"), "World!");
});

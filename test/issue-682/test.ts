import { assertEquals } from "jsr:@std/assert";
import {
  ArrayPrototypePush,
  eval as indirectEval,
} from "http://localhost:8080/@nodefill/primordials@4.1.0?target=es2022";

Deno.test("issue #682 - CommonJS eval export", () => {
  assertEquals(indirectEval("1 + 1"), 2);
  const values: number[] = [];
  ArrayPrototypePush(values, 42);
  assertEquals(values, [42]);
});

// close https://github.com/esm-dev/esm.sh/issues/741

import { assertEquals } from "jsr:@std/assert";

import { Kind, type TSchema, TypeRegistry } from "http://localhost:8080/@sinclair/typebox@0.32.22?no-bundle";
import { Value } from "http://localhost:8080/@sinclair/typebox@0.32.22/value?no-bundle";

Deno.test("issue #741", async () => {
  {
    const res = await fetch("http://localhost:8080/@sinclair/typebox@0.32.22?no-bundle");
    res.body?.cancel();
    assertEquals(res.ok, true);
    assertEquals(res.headers.get("x-typescript-types"), "http://localhost:8080/@sinclair/typebox@0.32.22/build/require/index.d.ts");
  }
  {
    const res = await fetch("http://localhost:8080/@sinclair/typebox@0.32.22/value?no-bundle");
    res.body?.cancel();
    assertEquals(res.ok, true);
    assertEquals(res.headers.get("x-typescript-types"), "http://localhost:8080/@sinclair/typebox@0.32.22/build/require/value/index.d.ts");
  }
  {
    const Foo = { [Kind]: "Foo" } as TSchema;
    TypeRegistry.Set("Foo", (_, value) => value === "foo");
    assertEquals(Value.Check(Foo, "foo"), true);
    assertEquals(Value.Check(Foo, "bar"), false);
  }
});

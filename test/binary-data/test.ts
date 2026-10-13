import { assert, assertEquals, assertStringIncludes } from "jsr:@std/assert";
import binaryData from "esm.sh/binary-data@0.6.0?target=denonext";

Deno.test("binary-data private modules", () => {
  const schema = {
    kind: binaryData.types.uint8,
    value: binaryData.types.uint8,
  };
  const value = { kind: 7, value: 42 };
  const packet = binaryData.encode(value, schema).slice();
  assertEquals([...packet], [7, 42]);
  assertEquals(binaryData.decode(packet, schema), value);
});

Deno.test("binary-data external wildcard", async () => {
  for (const external of ["*", "lib,*", "*,lib"]) {
    const res = await fetch(
      import.meta.resolve(
        `esm.sh/binary-data@0.6.0?target=denonext&external=${external}`,
      ),
    );
    await res.body?.cancel();
    assertEquals(res.status, 200);
    const modulePath = res.headers.get("x-esm-path");
    assert(modulePath);
    const build = await fetch(new URL(modulePath, res.url));
    const code = await build.text();
    assertEquals(build.status, 200);
    assertStringIncludes(code, 'from"lib/binary-stream"');
    assertStringIncludes(code, 'from"types/numbers"');
  }
});

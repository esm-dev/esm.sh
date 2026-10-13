import { assertEquals } from "jsr:@std/assert";
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

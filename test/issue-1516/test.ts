import { assertEquals } from "jsr:@std/assert";

import * as detect from "http://localhost:8080/wasm-feature-detect@1.10.0?target=es2022";
import * as detectDev from "http://localhost:8080/wasm-feature-detect@1.10.0?target=es2022&dev";

Deno.test("issue #1516 - guarded source phase import detection", async () => {
  let supported = false;
  try {
    await new Function(
      'return import.source("data:application/wasm;charset=utf-8;base64,AGFzbQEAAAAACARuYW1lAgEA")',
    )();
    supported = true;
  } catch {
    supported = false;
  }

  for (const mod of [detect, detectDev]) {
    assertEquals(await mod.sourcePhaseImports(), supported);
    assertEquals(await mod.simd(), true);
  }
});

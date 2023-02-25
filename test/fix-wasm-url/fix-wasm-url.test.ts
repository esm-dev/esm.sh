import { assertEquals } from "https://deno.land/std@0.178.0/testing/asserts.ts";

import init, {
  transform,
} from "http://localhost:8080/lightningcss-wasm@1.19.0";

Deno.test("Fix wasm URL", async () => {
  await init();
  const { code } = transform({
    filename: "style.css",
    code: new TextEncoder().encode(".foo { color: red }"),
    minify: true,
  });
  assertEquals(new TextDecoder().decode(code), ".foo{color:red}");
});

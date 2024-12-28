import { assertStringIncludes } from "jsr:@std/assert";

Deno.test("splitting sub-modules are shared by export modules", async () => {
  const res = await fetch("http://localhost:8080/svelte@5.16.0?target=es2022");
  const text = await res.text();
  assertStringIncludes(text, "src/internal/client/render.mjs");
});

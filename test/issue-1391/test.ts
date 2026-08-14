import { assertEquals } from "jsr:@std/assert";

// https://github.com/esm-dev/esm.sh/issues/1391
Deno.test("main field should not override the root export", async () => {
  const { __version__ } = await import(
    "http://localhost:8080/langsmith@0.8.10?target=es2022"
  );
  assertEquals(__version__, "0.8.10");
});

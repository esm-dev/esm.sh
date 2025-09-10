import { assert } from "jsr:@std/assert";

// related issue: https://github.com/esm-dev/esm.sh/issues/1199
Deno.test(
  "fix astring entry in browser",
  async () => {
    const mod = await import("http://localhost:8080/astring@1.9.0?target=es2022");
    assert(typeof mod.generate === "function");
  },
);

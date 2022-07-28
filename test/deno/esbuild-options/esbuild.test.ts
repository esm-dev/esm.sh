import {
  assertEquals,
  assertStringIncludes,
} from "https://deno.land/std@0.145.0/testing/asserts.ts";

Deno.test("esbuild options", async (t) => {
  await t.step("?sourcemap", async () => {
    const res = await fetch(
      `http://localhost:8080/v87/react-dom@18.2.0/deno/react-dom.sm.js`,
    );
    const code = await res.text();
    assertEquals(
      res.headers.get("content-type"),
      "application/javascript",
    );
    assertStringIncludes(
      code,
      "//# sourceMappingURL=data:application/json;base64",
    );
    assertStringIncludes(
      code,
      "/deno/react.sm.js",
    );
  });
});

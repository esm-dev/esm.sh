import {
  assertEquals,
  assertStringIncludes,
} from "https://deno.land/std@0.155.0/testing/asserts.ts";

Deno.test("esbuild options", async (t) => {
  await t.step("?sourcemap", async () => {
    const res = await fetch(
      `http://localhost:8080/react-dom@18.2.0?sourcemap`,
    );
    const code = await res.text();
    const m = code.match(/"(http:\/\/localhost:8080\/v.+?)"/);
    assertEquals(m?.length, 2);
    const res2 = await fetch(m?.[1]!);
    const code2 = await res2.text();
    assertStringIncludes(
      code2,
      "//# sourceMappingURL=data:application/json;base64",
    );
  });
});

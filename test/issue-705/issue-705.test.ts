import {
  assertEquals,
  assertStringIncludes,
} from "https://deno.land/std@0.180.0/testing/asserts.ts";

Deno.test("issue #705", async () => {
  const { version } = await fetch("http://localhost:8080/status.json").then((
    res,
  ) => res.json());
  const dts = await fetch(
    `http://localhost:8080/v${version}/shikiji@0.3.3/dist/index.d.mts`,
  ).then((res) => res.text());
  const { default: nord } = await import(
    `http://localhost:8080/v${version}/shikiji@0.3.3/es2022/dist/themes/nord.js`
  );
  assertStringIncludes(dts, "'./types/types.d.mts'");
  assertEquals(nord.name, "nord");
});

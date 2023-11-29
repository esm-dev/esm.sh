import { assertStringIncludes } from "https://deno.land/std@0.180.0/testing/asserts.ts";

Deno.test("issue #765", async () => {
  const { version } = await fetch("http://localhost:8080/status.json").then((
    res,
  ) => res.json());

  const dts = await fetch(
    `http://localhost:8080/v${version}/openai@v4.20.0/index.d.mts`,
  ).then((res) => res.text());

  assertStringIncludes(
    dts,
    `http://localhost:8080/v${version}/openai@4.20.0/resources/index.d.ts`,
  );

  const dts2 = await fetch(
    `http://localhost:8080/v${version}/openai@v4.20.0/resources/index.d.ts`,
  ).then((res) => res.text());

  assertStringIncludes(dts2, "ImagesResponse");
});

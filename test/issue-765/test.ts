import { assertStringIncludes } from "jsr:@std/assert";

Deno.test("issue #765", async () => {
  const dts = await fetch(
    `http://localhost:8080/openai@v4.20.0/index.d.mts`,
  ).then((res) => res.text());

  assertStringIncludes(
    dts,
    `http://localhost:8080/openai@4.20.0/resources/index.d.ts`,
  );

  const dts2 = await fetch(
    `http://localhost:8080/openai@v4.20.0/resources/index.d.ts`,
  ).then((res) => res.text());

  assertStringIncludes(dts2, "ImagesResponse");
});

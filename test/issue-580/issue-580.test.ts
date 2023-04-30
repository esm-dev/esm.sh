import {
  assertEquals,
  assertStringIncludes,
} from "https://deno.land/std@0.180.0/testing/asserts.ts";

Deno.test("issue #580", async () => {
  const { version } = await fetch("http://localhost:8080/status.json").then((
    res,
  ) => res.json());
  let res = await fetch(`http://localhost:8080/v${version}/pocketbase@0.13.1`);
  const dtsUrl =
    `http://localhost:8080/v${version}/pocketbase@0.13.1/dist/pocketbase.es.d.mts`;
  const dtsHeader = res.headers.get("x-typescript-types");
  res.body?.cancel();
  assertEquals(dtsHeader, dtsUrl);
  const dts = await fetch(dtsUrl).then((res) => res.text());
  assertStringIncludes(dts, "declare function getTokenPayload");
});

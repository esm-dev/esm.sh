import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("issue #580", async () => {
  let res = await fetch(`http://localhost:8080/pocketbase@0.13.1`);
  const dtsUrl = `http://localhost:8080/pocketbase@0.13.1/dist/pocketbase.es.d.mts`;
  const dtsHeader = res.headers.get("x-typescript-types");
  res.body?.cancel();
  assertEquals(dtsHeader, dtsUrl);
  const dts = await fetch(dtsUrl).then((res) => res.text());
  assertStringIncludes(dts, "declare function getTokenPayload");
});

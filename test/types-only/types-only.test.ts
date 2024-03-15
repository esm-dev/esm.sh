import { assertEquals, assertStringIncludes } from "https://deno.land/std@0.220.0/assert/mod.ts";

Deno.test("types only", async () => {
  const res = await fetch(
    "http://localhost:8080/@octokit-next/types-rest-api@2.5.0",
  );
  res.body?.cancel();
  assertEquals(res.status, 200);
  assertEquals(
    res.headers.get("content-type"),
    "application/javascript; charset=utf-8",
  );
  const dtsUrl = `http://localhost:8080/@octokit-next/types-rest-api@2.5.0/index.d.ts`;
  assertEquals(res.headers.get("x-typescript-types"), dtsUrl);
  const dts = await fetch(dtsUrl).then((r) => r.text());
  assertStringIncludes(dts, `declare module "http://localhost:8080/@octokit-next/types@2.5.0/index.d.ts"`);
});

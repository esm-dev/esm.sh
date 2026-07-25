import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("types only", async () => {
  const res = await fetch(
    "http://localhost:8080/@octokit-next/types-rest-api@2.5.0?origin-regression",
    {
      headers: {
        "X-Real-Origin": `https://attacker.invalid" } = options; globalThis.PWNED = true; const { z = "`,
      },
    },
  );
  res.body?.cancel();
  assertEquals(res.status, 200);
  assertEquals(res.headers.get("content-type"), "application/javascript; charset=utf-8");
  const dtsUrl = `/@octokit-next/types-rest-api@2.5.0/index.d.ts`;
  assertEquals(res.headers.get("x-typescript-types"), dtsUrl);
  const dts = await fetch(new URL(dtsUrl, res.url), {
    headers: {
      "X-Real-Origin": "https://attacker.invalid",
    },
  }).then((r) => r.text());
  assertStringIncludes(dts, `declare module "/@octokit-next/types@2.5.0/index.d.ts"`);
});

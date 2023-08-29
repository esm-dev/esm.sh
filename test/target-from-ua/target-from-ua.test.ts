import { assertEquals ,assertStringIncludes} from "https://deno.land/std@0.180.0/testing/asserts.ts";

Deno.test("build target from UA", async () => {
  const getBuildId = async (ua: string) => {
    const res = await fetch("http://localhost:8080/preact@10.10.6", {
      headers: { "User-Agent": ua },
    });
    res.body?.cancel()
    return res.headers.get("x-esm-id")!
  };
  assertStringIncludes(await getBuildId("curl/7.86.0"), "/esnext/");
  assertStringIncludes(await getBuildId("Deno/1.33.1"), "/deno/");
  assertStringIncludes(await getBuildId("Deno/1.33.2"), "/denonext/");
  assertStringIncludes(await getBuildId("HeadlessChrome/108.0.4512.0"), "/chrome108/");
  assertStringIncludes(await getBuildId("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Safari/537.36"), "/chrome116/");
  assertStringIncludes(await getBuildId("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.3 Safari/605.1.15"), "/safari16.3/");
  assertStringIncludes(await getBuildId("Mozilla/5.0 (iPhone; CPU iPhone OS 13_2_3 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/13.0.3 Mobile/15E148 Safari/604.1"), "/ios13.0/");
  assertStringIncludes(await getBuildId("Mozilla/5.0 (iPad; CPU OS 13_3 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/87.0.4280.77 Mobile/15E148 Safari/604.1"), "/chrome87/");
})

Deno.test("esma target from UA", async () => {
  const getTarget = async (ua: string) => {
    const rest = await fetch("http://localhost:8080/esma-target", {
      headers: { "User-Agent": ua },
    });
    return await rest.text();
  };
  assertEquals(await getTarget("Deno/1.33.1"), "deno");
  assertEquals(await getTarget("Deno/1.33.2"), "denonext");
  assertEquals(await getTarget("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Safari/537.36"), "es2022");
  assertEquals(await getTarget("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.3 Safari/605.1.15"), "es2021");
});

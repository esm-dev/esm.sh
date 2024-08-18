import { assertEquals } from "jsr:@std/assert";

Deno.test("build target from UA", async () => {
  const getTarget = async (ua: string) => {
    const rest = await fetch("http://localhost:8080/esma-target", {
      headers: { "User-Agent": ua },
    });
    return await rest.text();
  };
  assertEquals(await getTarget("Deno/1.33.1"), "deno");
  assertEquals(await getTarget("Deno/1.33.2"), "denonext");
  assertEquals(await getTarget("HeadlessChrome/109"), "es2021");
  assertEquals(
    await getTarget(
      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Safari/537.36",
    ),
    "es2024",
  );
  assertEquals(
    await getTarget(
      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.3 Safari/605.1.15",
    ),
    "es2021",
  );
});

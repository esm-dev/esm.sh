import { assertEquals } from "https://deno.land/std@0.180.0/testing/asserts.ts";

Deno.test("deno target", async () => {
  const getTarget = async (ua: string) => {
    const rest = await fetch(`http://localhost:8080/esma-target`, {
      headers: { "User-Agent": ua },
    });
    return await rest.text();
  };
  assertEquals(await getTarget("Deno/1.33.1"), "deno");
  assertEquals(await getTarget("Deno/1.33.2"), "denonext");
});

import { assertEquals, assertStringIncludes } from "https://deno.land/std@0.210.0/testing/asserts.ts";

Deno.test("`?deno-std` query", async () => {
  const res = await fetch(
    `http://localhost:8080/typescript@5.4.2?target=deno&deno-std=0.128.0`,
  );
  res.body?.cancel();
  assertEquals(res.status, 200);
  const esmId = res.headers.get("x-esm-id");
  assertStringIncludes(
    await fetch(`http://localhost:8080/${esmId}`).then((res) => res.text()),
    "https://deno.land/std@0.128.0/",
  );
});

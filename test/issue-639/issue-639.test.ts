import { assertEquals } from "https://deno.land/std@0.210.0/testing/asserts.ts";

Deno.test("issue #639", async () => {
  const res = await fetch(
    `http://localhost:8080/robodux@15.0.0/dist/index.d.ts`,
    { redirect: "manual" },
  );
  assertEquals(res.status, 200);
  assertEquals(
    res.headers.get("content-type")!,
    "application/typescript; charset=utf-8",
  );
  res.body?.cancel();
});

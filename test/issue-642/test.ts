import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("issue #642", async () => {
  const res = await fetch(
    `http://localhost:8080/async-mutex@0.4.0/lib/Mutex.d.ts`,
    { redirect: "manual" },
  );
  assertEquals(res.status, 200);
  assertEquals(
    res.headers.get("content-type")!,
    "application/typescript; charset=utf-8",
  );
  assertStringIncludes(await res.text(), "./MutexInterface.d.ts");
});

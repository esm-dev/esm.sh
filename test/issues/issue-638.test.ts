import { assertEquals } from "jsr:@std/assert";

Deno.test("issue #638", async () => {
  const res = await fetch(
    `http://localhost:8080/@sqlite.org/sqlite-wasm@3.41.2/es2022/sqlite3.wasm`,
    { redirect: "manual" },
  );
  assertEquals(res.status, 301);
  assertEquals(
    res.headers.get("location")!,
    "http://localhost:8080/@sqlite.org/sqlite-wasm@3.41.2/sqlite-wasm/jswasm/sqlite3.wasm",
  );
  res.body?.cancel();
});

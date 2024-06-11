import { assert, assertEquals } from "https://deno.land/std@0.220.0/assert/mod.ts";

Deno.test("package css", async () => {
  const res = await fetch("http://localhost:8080/monaco-editor@0.40.0?css&target=es2022");
  assert(res.redirected);
  assertEquals(res.headers.get("content-type"), "text/css; charset=utf-8");
  res.body?.cancel();

  const res2 = await fetch("http://localhost:8080/monaco-editor@0.40.0/es2022/monaco-editor.css");
  assert(!res2.redirected);
  assertEquals(res2.headers.get("content-type"), "text/css; charset=utf-8");
  res2.body?.cancel();
});

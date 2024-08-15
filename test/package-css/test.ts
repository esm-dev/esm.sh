import { assert, assertEquals } from "jsr:@std/assert";

Deno.test("package css", async () => {
  const res = await fetch("http://localhost:8080/monaco-editor@0.40.0?css&target=es2022", { redirect: "manual" });
  assertEquals(res.status, 301);
  assertEquals(res.headers.get("location"), "http://localhost:8080/monaco-editor@0.40.0/es2022/monaco-editor.css");
  res.body?.cancel();

  const res2 = await fetch("http://localhost:8080/monaco-editor@0.40.0/es2022/monaco-editor.css", { redirect: "manual" });
  assert(!res2.redirected);
  assertEquals(res2.headers.get("content-type"), "text/css; charset=utf-8");
  res2.body?.cancel();
});

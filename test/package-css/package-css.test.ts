import { assertEquals } from "https://deno.land/std@0.180.0/testing/asserts.ts";

Deno.test("package css", async () => {
  const { version } = await fetch("http://localhost:8080/status.json").then((
    res,
  ) => res.json());
  const res = await fetch(
    "http://localhost:8080/monaco-editor@0.36.1?css&target=es2022",
  );
  assertEquals(
    res.url,
    `http://localhost:8080/v${version}/monaco-editor@0.36.1/es2022/monaco-editor.css`,
  );
  assertEquals(res.headers.get("content-type"), "text/css; charset=utf-8");
  res.body?.cancel();
});

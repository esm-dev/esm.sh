import { assertStringIncludes } from "https://deno.land/std@0.180.0/testing/asserts.ts";

Deno.test("issue #694", async () => {
  const res = await fetch(
    "http://localhost:8080/monaco-editor@0.40.0/esm/vs/editor/editor.worker?target=es2022&bundle",
  );
  await res.body?.cancel();
  const id = res.headers.get("x-esm-id");
  const code = await fetch(
    `http://localhost:8080/${id}`,
  ).then((res) => res.text());
  assertStringIncludes(code, `import __Process$ from "data:text/javascript;`);
});

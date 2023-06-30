import { assertStringIncludes } from "https://deno.land/std@0.180.0/testing/asserts.ts";

Deno.test("issue #671", async () => {
  const res = await fetch(
    "http://localhost:8080/flowbite-react@v0.4.9?alias=react:preact/compat,react-dom:preact/compat",
  );
  await res.body?.cancel();
  const id = res.headers.get("x-esm-id");
  const code = await fetch(
    `http://localhost:8080/${id}`,
  ).then((res) => res.text());
  assertStringIncludes(code, "compat/jsx-runtime.js");
});

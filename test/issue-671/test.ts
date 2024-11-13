import { assertStringIncludes } from "jsr:@std/assert";

Deno.test("issue #671", async () => {
  const res = await fetch(
    "http://localhost:8080/flowbite-react@v0.4.9?alias=react:preact/compat,react-dom:preact/compat",
  );
  await res.body?.cancel();
  const esmPath = res.headers.get("x-esm-path");
  const code = await fetch("http://localhost:8080" + esmPath).then((res) => res.text());
  assertStringIncludes(code, 'from"/preact/compat/jsx-runtime?target=denonext"');
  assertStringIncludes(code, 'from"/preact/compat?target=denonext"');
  assertStringIncludes(code, 'hi?alias=react:preact/compat&target=denonext"');

  const res2 = await fetch(
    "http://localhost:8080/flowbite-react@v0.4.9?alias=react:preact/compat,react-dom:preact/compat&deps=preact@10.0.0&target=es2020",
  );
  await res2.body?.cancel();
  const esmPath2 = res2.headers.get("x-esm-path");
  const code2 = await fetch("http://localhost:8080" + esmPath2).then((res) => res.text());
  assertStringIncludes(code2, 'from"/preact@10.0.0/es2020/compat/jsx-runtime.js"');
  assertStringIncludes(code2, 'from"/preact@10.0.0/es2020/compat.js"');
  assertStringIncludes(code2, 'hi?alias=react:preact/compat&deps=preact@10.0.0&target=es2020"');
});

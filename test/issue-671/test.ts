import { assert, assertStringIncludes } from "jsr:@std/assert";

Deno.test("issue #671", async () => {
  const res = await fetch(
    "http://localhost:8080/flowbite-react@v0.4.9?alias=react:preact/compat,react-dom:preact/compat",
  );
  await res.body?.cancel();
  const esmPath = res.headers.get("x-esm-path");
  const code = await fetch("http://localhost:8080" + esmPath).then((res) =>
    res.text()
  );
  const match = code.match(
    /from"\/preact@([^/]+)\/denonext\/compat\/jsx-runtime\.mjs"/,
  );
  assert(match);
  const version = match[1];
  assertStringIncludes(code, `from"/preact@${version}/denonext/compat.mjs"`);
  assertStringIncludes(
    code,
    `"/react-icons@^4.10.1/hi?alias=${
      encodeURIComponent(`react:preact@${version}/compat`)
    }&target=denonext"`,
  );

  const res2 = await fetch(
    "http://localhost:8080/flowbite-react@v0.4.9?alias=react:preact/compat,react-dom:preact/compat&deps=preact@10.0.0&target=es2020",
  );
  await res2.body?.cancel();
  const esmPath2 = res2.headers.get("x-esm-path");
  const code2 = await fetch("http://localhost:8080" + esmPath2).then((res) =>
    res.text()
  );
  assertStringIncludes(
    code2,
    'from"/preact@10.0.0/es2020/compat/jsx-runtime.mjs"',
  );
  assertStringIncludes(code2, 'from"/preact@10.0.0/es2020/compat.mjs"');
  assertStringIncludes(
    code2,
    'hi?alias=react%3Apreact%4010.0.0%2Fcompat&deps=preact%4010.0.0&target=es2020"',
  );
});

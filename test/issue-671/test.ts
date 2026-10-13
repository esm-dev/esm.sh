import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("issue #671", async () => {
  const res = await fetch(
    "http://localhost:8080/flowbite-react@v0.4.9?alias=react:preact/compat,react-dom:preact/compat",
  );
  await res.body?.cancel();
  const esmPath = res.headers.get("x-esm-path");
  const code = await fetchModuleCode("http://localhost:8080" + esmPath);
  assertStringIncludes(code, 'from"/preact/compat/jsx-runtime?target=denonext"');
  assertStringIncludes(code, 'from"/preact/compat?target=denonext"');
  assertStringIncludes(code, '"/react-icons@^4.10.1/hi?alias=react:preact/compat&target=denonext"');

  const res2 = await fetch(
    "http://localhost:8080/flowbite-react@v0.4.9?alias=react:preact/compat,react-dom:preact/compat&deps=preact@10.0.0&target=es2020",
  );
  await res2.body?.cancel();
  const esmPath2 = res2.headers.get("x-esm-path");
  const code2 = await fetchModuleCode("http://localhost:8080" + esmPath2);
  assertStringIncludes(code2, 'from"/preact@10.0.0/es2020/compat/jsx-runtime.mjs"');
  assertStringIncludes(code2, 'from"/preact@10.0.0/es2020/compat.mjs"');
  assertStringIncludes(code2, 'hi?alias=react:preact/compat&deps=preact@10.0.0&target=es2020"');
});

async function fetchModuleCode(entry: string): Promise<string> {
  const urls = new Set([entry]);
  let code = "";
  for (const url of urls) {
    const res = await fetch(url);
    assertEquals(res.status, 200);
    const source = await res.text();
    code += "\n" + source;
    for (const [, specifier] of source.matchAll(/\bfrom\s*["'](\.[^"']+)["']/g)) {
      urls.add(new URL(specifier, url).href);
    }
  }
  return code;
}

import { assert, assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("query as version suffix", async () => {
  const res = await fetch("http://localhost:8080/react-dom@18.3.1&dev&target=es2022&deps=react@18.3.1/client");
  const code = await res.text();
  assertEquals(res.status, 200);
  assertEquals(res.headers.get("cache-control"), "public, max-age=31536000, immutable");
  assertEquals(res.headers.get("content-type"), "application/javascript; charset=utf-8");
  assert(!res.headers.get("vary")?.includes("User-Agent"));
  assertStringIncludes(code, "/react-dom@18.3.1/X-ZHJlYWN0QDE4LjMuMQ/es2022/client.development.mjs");
});

Deno.test("`/jsx-runtime` in query", async () => {
  const res = await fetch("http://localhost:8080/react@18.3.1?dev&target=es2022/jsx-runtime");
  const code = await res.text();
  assertEquals(res.status, 200);
  assertEquals(res.headers.get("cache-control"), "public, max-age=31536000, immutable");
  assertEquals(res.headers.get("content-type"), "application/javascript; charset=utf-8");
  assert(!res.headers.get("vary")?.includes("User-Agent"));
  assertStringIncludes(code, "/react@18.3.1/es2022/jsx-runtime.development.mjs");
});

Deno.test("redirect semantic versioning module for deno target", async () => {
  "deno target";
  {
    const res = await fetch("http://localhost:8080/preact", { redirect: "manual" });
    res.body?.cancel();
    assertEquals(res.status, 302);
    assertEquals(res.headers.get("cache-control"), "public, max-age=600");
    assertStringIncludes(res.headers.get("location")!, "http://localhost:8080/preact@");
    assertStringIncludes(res.headers.get("vary") ?? "", "User-Agent");
  }

  "browser target";
  {
    const res = await fetch("http://localhost:8080/preact", { redirect: "manual", headers: { "User-Agent": "ES/2022" } });
    const code = await res.text();
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("cache-control"), "public, max-age=600");
    assertEquals(res.headers.get("content-type"), "application/javascript; charset=utf-8");
    assertStringIncludes(res.headers.get("vary") ?? "", "User-Agent");
    assertStringIncludes(code, "/preact@");
    assertStringIncludes(code, "/es2022/");
  }
});

Deno.test("redirect asset URLs", async () => {
  {
    const res = await fetch("http://localhost:8080/react/package.json", { redirect: "manual" });
    res.body?.cancel();
    assertEquals(res.status, 302);
    assertEquals(res.headers.get("cache-control"), "public, max-age=600");
    assertStringIncludes(res.headers.get("location")!, "http://localhost:8080/react@");

    const res2 = await fetch(res.headers.get("location")!, { redirect: "manual" });
    const pkg = await res2.json();
    assertEquals(res2.status, 200);
    assertEquals(res2.headers.get("cache-control"), "public, max-age=31536000, immutable");
    assertStringIncludes(pkg.name, "react");
  }
  {
    const res = await fetch("http://localhost:8080/react@^18.3.1/package.json", { redirect: "manual" });
    res.body?.cancel();
    assertEquals(res.status, 302);
    assertEquals(res.headers.get("cache-control"), "public, max-age=600");
    assertStringIncludes(res.headers.get("location")!, "http://localhost:8080/react@18.");

    const res2 = await fetch(res.headers.get("location")!, { redirect: "manual" });
    const pkg = await res2.json();
    assertEquals(res2.status, 200);
    assertEquals(res2.headers.get("cache-control"), "public, max-age=31536000, immutable");
    assertStringIncludes(pkg.name, "react");
  }
  {
    const res = await fetch("http://localhost:8080/react/package.json?module", { redirect: "manual" });
    res.body?.cancel();
    assertEquals(res.status, 302);
    assertEquals(res.headers.get("cache-control"), "public, max-age=600");
    assertStringIncludes(res.headers.get("location")!, "http://localhost:8080/react@");
    assert(res.headers.get("location")!.endsWith("/package.json?module"));

    const res2 = await fetch(res.headers.get("location")!, { redirect: "manual" });
    const js = await res2.text();
    assertEquals(res2.status, 200);
    assertEquals(res2.headers.get("cache-control"), "public, max-age=31536000, immutable");
    assertEquals(res2.headers.get("content-type"), "application/javascript; charset=utf-8");
    assertStringIncludes(js, "export default");
  }
  {
    const res = await fetch("http://localhost:8080/@lezer/highlight@1.2.1?raw", { redirect: "manual" });
    res.body?.cancel();
    assertEquals(res.status, 301);
    assertEquals(res.headers.get("cache-control"), "public, max-age=31536000, immutable");
    assertEquals(res.headers.get("location")!, "http://localhost:8080/@lezer/highlight@1.2.1/dist/index.js?raw");
  }
});

Deno.test("Fix wasm URLs with `target` segment", async () => {
  {
    const res = await fetch(
      "http://localhost:8080/lightningcss-wasm@1.19.0/deno/lightningcss_node.wasm",
      { redirect: "manual" },
    );
    res.body?.cancel();
    assertEquals(res.status, 301);
    assertEquals(
      res.headers.get("location"),
      "http://localhost:8080/lightningcss-wasm@1.19.0/lightningcss_node.wasm",
    );
  }
  {
    const res = await fetch(
      "http://localhost:8080/esm-compiler@0.7.2/es2024/esm_compiler_bg.wasm",
      { redirect: "manual" },
    );
    res.body?.cancel();
    assertEquals(res.status, 301);
    assertEquals(
      res.headers.get("location"),
      "http://localhost:8080/esm-compiler@0.7.2/pkg/esm_compiler_bg.wasm",
    );
  }
  {
    const res = await fetch(
      "http://localhost:8080/gh/oxc-project/oxc@7d785c3/es2022/napi/parser/parser.wasm32-wasi.wasm",
      { redirect: "manual" },
    );
    res.body?.cancel();
    assertEquals(res.status, 301);
    assertEquals(
      res.headers.get("location"),
      "http://localhost:8080/gh/oxc-project/oxc@7d785c3/napi/parser/parser.wasm32-wasi.wasm",
    );
  }
});

Deno.test("Fix json URLs with `target` segment", async () => {
  const res = await fetch(
    "http://localhost:8080/lightningcss-wasm@1.19.0/deno/package.json",
    { redirect: "manual" },
  );
  res.body?.cancel();
  assertEquals(res.status, 301);
  assertEquals(
    res.headers.get("location"),
    "http://localhost:8080/lightningcss-wasm@1.19.0/package.json",
  );
});

Deno.test("support `/#/` in path", async () => {
  const res = await fetch("http://localhost:8080/es5-ext@0.10.50/string/%23/contains");
  assertEquals(res.status, 200);
  assertEquals(res.headers.get("content-type"), "application/javascript; charset=utf-8");
  assertEquals(res.headers.get("cache-control"), "public, max-age=31536000, immutable");
  assertStringIncludes(await res.text(), "/denonext/string/%23/contains.mjs");
});

Deno.test("dts-transformer: support `.d` extension", async () => {
  const res = await fetch("http://localhost:8080/tailwindcss@3.3.5/types/index.d.ts");
  const dts = await res.text();
  assertStringIncludes(dts, "'./config.d.ts'");
});

Deno.test("fix rewritten build path", async () => {
  for (let i = 0; i < 2; i++) {
    const res = await fetch("http://localhost:8080/@ucanto/core@10.0.1/denonext/src/lib.mjs");
    const js = await res.text();
    assertEquals(js.trim(), `export * from "/@ucanto/core@10.0.1/denonext/core.mjs";`);
  }
});

Deno.test("redirect to css entry", async () => {
  const res = await fetch("http://localhost:8080/@markprompt/css@0.33.0", { redirect: "manual" });
  res.body?.cancel();
  assertEquals(res.status, 301);
  assertEquals(res.headers.get("location"), "http://localhost:8080/@markprompt/css@0.33.0/markprompt.css");
});

Deno.test("[workaround] force the dependency version of react equals to react-dom", async () => {
  const res = await fetch("http://localhost:8080/react-dom@18.2.0?target=es2022");
  assertEquals(res.status, 200);
  const js = await res.text();
  assertStringIncludes(js, '"/react@18.2.0/es2022/react.mjs"');
});

Deno.test("fix external save path", async () => {
  // non-external module
  {
    const res = await fetch("http://localhost:8080/preact@10.25.4/es2022/hooks.mjs");
    assertStringIncludes(await res.text(), 'from"./preact.mjs"');
  }
  // in https://github.com/preactjs/preact-www/issues/1225, the module returns the non-external module
  {
    const res = await fetch("http://localhost:8080/*preact@10.25.4/es2022/hooks.mjs");
    assertStringIncludes(await res.text(), 'from"preact"');
  }
});

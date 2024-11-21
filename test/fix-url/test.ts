import { assert, assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("query as version suffix", async () => {
  const res = await fetch("http://localhost:8080/react-dom@18.3.1&dev&target=es2022&deps=react@18.3.1/client");
  const code = await res.text();
  assertEquals(res.status, 200);
  assertEquals(res.headers.get("cache-control"), "public, max-age=31536000, immutable");
  assertEquals(res.headers.get("content-type"), "application/javascript; charset=utf-8");
  assert(!res.headers.get("vary")!.includes("User-Agent"));
  assertStringIncludes(code, "/react-dom@18.3.1/es2022/client.development.mjs");
});

Deno.test("`/jsx-runtime` in query", async () => {
  const res = await fetch("http://localhost:8080/react@18.3.1?dev&target=es2022/jsx-runtime");
  const code = await res.text();
  assertEquals(res.status, 200);
  assertEquals(res.headers.get("cache-control"), "public, max-age=31536000, immutable");
  assertEquals(res.headers.get("content-type"), "application/javascript; charset=utf-8");
  assert(!res.headers.get("vary")!.includes("User-Agent"));
  assertStringIncludes(code, "/react@18.3.1/es2022/jsx-runtime.development.mjs");
});

Deno.test("redirect semantic versioning module for deno target", async () => {
  "deno target";
  {
    const res = await fetch("http://localhost:8080/preact", { redirect: "manual" });
    res.body?.cancel();
    assertEquals(res.status, 302);
    assertEquals(res.headers.get("cache-control"), "public, max-age=3600");
    assertStringIncludes(res.headers.get("location")!, "http://localhost:8080/preact@");
    assertStringIncludes(res.headers.get("vary")!, "User-Agent");
  }

  "browser target";
  {
    const res = await fetch("http://localhost:8080/preact", { redirect: "manual", headers: { "User-Agent": "ES/2022" } });
    const code = await res.text();
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("cache-control"), "public, max-age=3600");
    assertEquals(res.headers.get("content-type"), "application/javascript; charset=utf-8");
    assertStringIncludes(res.headers.get("vary")!, "User-Agent");
    assertStringIncludes(code, "/preact@");
    assertStringIncludes(code, "/es2022/");
  }
});

Deno.test("redirect asset URLs", async () => {
  const res = await fetch("http://localhost:8080/preact/package.json", { redirect: "manual" });
  res.body?.cancel();
  assertEquals(res.status, 302);
  assertEquals(res.headers.get("cache-control"), "public, max-age=3600");
  assertStringIncludes(res.headers.get("location")!, "http://localhost:8080/preact@");

  const res2 = await fetch(res.headers.get("location")!, { redirect: "manual" });
  const pkg2 = await res2.json();
  assertEquals(res2.status, 200);
  assertEquals(res2.headers.get("cache-control"), "public, max-age=31536000, immutable");
  assertStringIncludes(pkg2.name, "preact");

  const res3 = await fetch("http://localhost:8080/preact@10/package.json", { redirect: "manual" });
  res3.body?.cancel();
  assertEquals(res3.status, 302);
  assertEquals(res3.headers.get("cache-control"), "public, max-age=3600");
  assertStringIncludes(res3.headers.get("location")!, "http://localhost:8080/preact@10.");

  const res4 = await fetch(res.headers.get("location")!, { redirect: "manual" });
  const pkg4 = await res4.json();
  assertEquals(res4.status, 200);
  assertEquals(res4.headers.get("cache-control"), "public, max-age=31536000, immutable");
  assertStringIncludes(pkg4.name, "preact");
});

Deno.test("Fix wasm URLs with `target` segment", async () => {
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

  const res2 = await fetch(
    "http://localhost:8080/esm-compiler@0.7.2/es2024/esm_compiler_bg.wasm",
    { redirect: "manual" },
  );
  res2.body?.cancel();
  assertEquals(res2.status, 301);
  assertEquals(
    res2.headers.get("location"),
    "http://localhost:8080/esm-compiler@0.7.2/pkg/esm_compiler_bg.wasm",
  );
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

Deno.test("legacy routes", async () => {
  {
    const { esm, build, transform } = await import("http://localhost:8080/build");
    assertEquals(typeof esm, "function");
    assertEquals(typeof build, "function");
    assertEquals(typeof transform, "function");
    try {
      esm``;
    } catch (err) {
      assertStringIncludes(err.message, "deprecated");
    }
    try {
      await import("http://localhost:8080/");
    } catch (err) {
      assertStringIncludes(err.message, "deprecated");
    }
    try {
      await import("http://localhost:8080/v135");
    } catch (err) {
      assertStringIncludes(err.message, "deprecated");
    }
  }
  {
    const res = await fetch("http://localhost:8080/stable/react@18.3.1/es2022/react.mjs", {
      headers: { "User-Agent": "i'm a browser" },
    });
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertStringIncludes(await res.text(), "createElement");
  }
  {
    const res = await fetch("http://localhost:8080/v135/react-dom@18.3.1/es2022/client.js", {
      headers: { "User-Agent": "i'm a browser" },
    });
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertStringIncludes(await res.text(), "createRoot");
  }
  {
    const res = await fetch("http://localhost:8080/v64/many-keys-weakmap@1.0.0/es2022/many-keys-weakmap.js", {
      headers: { "User-Agent": "i'm a browser" },
    });
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertStringIncludes(await res.text(), "ManyKeysWeakMap");
  }
  {
    const res = await fetch("http://localhost:8080/v136/react-dom@18.3.1/es2022/client.js", {
      headers: { "User-Agent": "i'm a browser" },
    });
    res.body?.cancel();
    assertEquals(res.status, 400);
  }
  {
    const res = await fetch("http://localhost:8080/react-dom@18.3.1/es2022/client.js", {
      headers: { "User-Agent": "i'm a browser" },
    });
    res.body?.cancel();
    assertEquals(res.status, 404);
  }
});

import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("redirects", async () => {
  const res = await fetch("http://localhost:8080/react/package.json", { redirect: "manual" });
  res.body?.cancel();
  assertEquals(res.status, 302);
  assertEquals(res.headers.get("cache-control"), "public, max-age=600");
  assertStringIncludes(res.headers.get("location")!, "http://localhost:8080/react@");

  const res_ = await fetch(res.headers.get("location")!, { redirect: "manual" });
  assertEquals(res_.status, 200);
  assertEquals(res_.headers.get("cache-control"), "public, max-age=31536000, immutable");
  assertStringIncludes((await res_.json()).name, "react");

  const res2 = await fetch("http://localhost:8080/react", { redirect: "manual" });
  assertEquals(res2.status, 200);
  assertEquals(res2.headers.get("cache-control"), "public, max-age=600");
  assertStringIncludes(await res2.text(), "/react@");

  const res3 = await fetch("http://localhost:8080/react@18", { redirect: "manual" });
  assertEquals(res3.status, 200);
  assertEquals(res3.headers.get("cache-control"), "public, max-age=600");
  assertStringIncludes(await res3.text(), "/react@18.");

  const res4 = await fetch("http://localhost:8080/react@18.3.1", { redirect: "manual" });
  assertEquals(res4.status, 200);
  assertEquals(res4.headers.get("cache-control"), "public, max-age=31536000, immutable");
  assertStringIncludes(await res4.text(), "/react@18.3.1");
});

Deno.test("Fix wasm URL", async () => {
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

Deno.test("Fix json URL", async () => {
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

Deno.test("dts-transformer: support `.d` extension", async () => {
  const res = await fetch("http://localhost:8080/tailwindcss@3.3.5/types/index.d.ts");
  const dts = await res.text();
  assertStringIncludes(dts, "'./config.d.ts'");
});

import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("pkg.pr.new", async () => {
  const res = await fetch("http://localhost:8080/pr/tinybench@a832a55", { headers: { "user-agent": "i'm a browser" } });
  assertEquals(res.status, 200);
  assertEquals(res.headers.get("content-type"), "application/javascript; charset=utf-8");
  assertEquals(res.headers.get("cache-control"), "public, max-age=31536000, immutable");
  assertEquals(res.headers.get("x-typescript-types"), "http://localhost:8080/pr/tinybench@a832a55/dist/index.d.ts");
  const text = await res.text();
  assertStringIncludes(text, "/es2022/tinybench.mjs");
});

Deno.test("pkg.pr.new dts", async () => {
  const res = await fetch("http://localhost:8080/pr/tinybench@a832a55/dist/index.d.ts");
  assertEquals(res.status, 200);
  assertEquals(res.headers.get("content-type"), "application/typescript; charset=utf-8");
  assertEquals(res.headers.get("cache-control"), "public, max-age=31536000, immutable");
  const text = await res.text();
  assertStringIncludes(text, "declare class Bench extends EventTarget {");
});

Deno.test("pkg.pr.new routing", async () => {
  {
    const { Bench } = await import("http://localhost:8080/pr/tinybench@a832a55");
    assertEquals(typeof Bench, "function");
  }
  {
    const { Bench } = await import("http://localhost:8080/pr/tinylibs/tinybench/tinybench@a832a55");
    assertEquals(typeof Bench, "function");
  }
  {
    const { Bench } = await import("http://localhost:8080/pkg.pr.new/tinybench@a832a55");
    assertEquals(typeof Bench, "function");
  }
});

Deno.test("access pkg.pr.new raw files", async () => {
  const { name } = await fetch("http://localhost:8080/pr/tinylibs/tinybench/tinybench@a832a55/package.json").then((res) => res.json());
  assertEquals(name, "tinybench");
});

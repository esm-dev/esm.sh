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
    const { Bench } = await import("http://localhost:8080/pr/tinylibs/tinybench@a832a55");
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
  {
    const { proxy } = await import("http://localhost:8080/pr/valtio@main");
    assertEquals(typeof proxy, "function");
  }
  {
    const { proxy } = await import("http://localhost:8080/pr/valtio@e21edb3");
    assertEquals(typeof proxy, "function");
  }
  {
    const { proxy } = await import("http://localhost:8080/pr/pmndrs/valtio@e21edb3");
    assertEquals(typeof proxy, "function");
  }
  {
    const { proxy } = await import("http://localhost:8080/pr/pmndrs/valtio/valtio@e21edb3");
    assertEquals(typeof proxy, "function");
  }
  {
    const { defineComponent } = await import("http://localhost:8080/pr/vuejs/vue-vapor/@vue/runtime-dom@3f6ce96");
    assertEquals(typeof defineComponent, "function");
  }
});

Deno.test("access pkg.pr.new raw files", async () => {
  const { name } = await fetch("http://localhost:8080/pr/tinylibs/tinybench/tinybench@a832a55/package.json").then((res) => res.json());
  assertEquals(name, "tinybench");
});

Deno.test("pkg.pr.new as a dependency", async () => {
  const res = await fetch("http://localhost:8080/pr/vuejs/vue-vapor/vue@3f6ce96", { headers: { "user-agent": "i'm a browser" } });
  assertEquals(res.status, 200);
  assertEquals(res.headers.get("content-type"), "application/javascript; charset=utf-8");
  assertEquals(res.headers.get("cache-control"), "public, max-age=31536000, immutable");
  const codde = await res.text();
  assertStringIncludes(codde, 'import "/pr/vuejs/vue-vapor/@vue/runtime-dom@3f6ce96/es2022/runtime-dom.mjs"');
});

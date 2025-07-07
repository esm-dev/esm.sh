import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("tgz", async () => {
  const res = await fetch("http://localhost:8080/tgz/preact@https%3A%2F%2Fregistry.yarnpkg.com%2Fpreact%2F-%2Fpreact-10.26.6.tgz", { headers: { "user-agent": "i'm a browser" } });
  assertEquals(res.status, 200);
  assertEquals(res.headers.get("content-type"), "application/javascript; charset=utf-8");
  assertEquals(res.headers.get("cache-control"), "public, max-age=31536000, immutable");
  assertEquals(res.headers.get("x-typescript-types"), "http://localhost:8080/tgz/preact@https:%2F%2Fregistry.yarnpkg.com%2Fpreact%2F-%2Fpreact-10.26.6.tgz/src/index.d.ts");
  const text = await res.text();
  assertStringIncludes(text, "/es2022/preact.mjs");
});

Deno.test("tgz dts", async () => {
  const res = await fetch("http://localhost:8080/tgz/preact@https%3A%2F%2Fregistry.yarnpkg.com%2Fpreact%2F-%2Fpreact-10.26.6.tgz/src/index.d.ts");
  assertEquals(res.status, 200);
  assertEquals(res.headers.get("content-type"), "application/typescript; charset=utf-8");
  assertEquals(res.headers.get("cache-control"), "public, max-age=31536000, immutable");
  const text = await res.text();
  assertStringIncludes(text, "export abstract class Component<P, S> {");
});

Deno.test("tgz routing", async () => {
  {
    const { h } = await import("http://localhost:8080/tgz/preact@https%3A%2F%2Fregistry.yarnpkg.com%2Fpreact%2F-%2Fpreact-10.26.6.tgz");
    assertEquals(typeof h, "function");
  }
  {
    const { h } = await import("http://localhost:8080/tgz/preact@https:%2F%2Fregistry.yarnpkg.com%2Fpreact%2F-%2Fpreact-10.26.6.tgz");
    assertEquals(typeof h, "function");
  }
});

Deno.test("access tgz raw files", async () => {
  const { name } = await fetch("http://localhost:8080/tgz/preact@https:%2F%2Fregistry.yarnpkg.com%2Fpreact%2F-%2Fpreact-10.26.6.tgz/package.json").then((res) => res.json());
  assertEquals(name, "preact");
});

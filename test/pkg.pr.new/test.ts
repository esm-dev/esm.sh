import { assert, assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("routing", async () => {
  {
    const {mount}= await import("http://localhost:8080/pkg.pr.new/sveltejs/svelte@main");
    assertEquals(typeof mount, "function");
  }
  {
    const { mount } = await import("http://localhost:8080/pr/sveltejs/svelte@main");
    assertEquals(typeof mount, "function");
  }
});

Deno.test("resolve branch to commit hash", async () => {
  const res = await fetch("http://localhost:8080/pr/sveltejs/svelte@main", { headers: { "user-agent": "i'm a browser" } });
  assert(!res.redirected);
  assertEquals(res.status, 200);
  assertEquals(res.headers.get("content-type"), "application/javascript; charset=utf-8");
  assertEquals(res.headers.get("cache-control"), "public, max-age=600");
  const code = await res.text();
  assert(/sveltejs\/svelte@[\da-f]{7}/.test(code));
});


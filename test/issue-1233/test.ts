import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("issue #1233 - Svelte loader", async () => {
  {
    const res = await fetch(
      "http://localhost:8080/@onsvisual/svelte-components@1.1.20/dist/components/Section/Section.svelte",
      { headers: { "User-Agent": "i'm a browser" } },
    );
    const js = await res.text();
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("content-type"), "application/javascript; charset=utf-8");
    assertStringIncludes(js, '/js/utils?target=es2022');
  }
});

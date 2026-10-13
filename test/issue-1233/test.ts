import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

for (const bundle of [true, false]) {
  Deno.test(`issue #1233 - Svelte relative imports (bundle=${bundle})`, async () => {
    const url =
      "http://localhost:8080/@onsvisual/svelte-components@1.1.20/dist/components/Section/Section.svelte?target=es2022" +
      (bundle ? "" : "&bundle=false");
    const res = await fetch(url);
    const js = await res.text();
    assertEquals(res.status, 200);
    assertEquals(
      res.headers.get("content-type"),
      "application/javascript; charset=utf-8",
    );
    if (!bundle) {
      const prefix = "/@onsvisual/svelte-components@1.1.20/es2022/dist/";
      assertStringIncludes(
        js,
        prefix + "components/Container/Container.svelte.nobundle.mjs",
      );
      assertStringIncludes(js, prefix + "js/utils.nobundle.mjs");
    }
    const { default: Section } = await import(url);
    assertEquals(typeof Section, "function");
  });
}

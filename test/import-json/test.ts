import { assert, assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test(
  "bundle json",
  async () => {
    const res = await fetch("http://localhost:8080/cli-spinners@3.2.1/denonext/cli-spinners.mjs");
    const text = await res.text();
    assert(res.ok);
    assertEquals(res.headers.get("content-type"), "application/javascript; charset=utf-8");
    assertEquals(res.headers.get("cache-control"), "public, max-age=31536000, immutable");
    assertStringIncludes(text, `dots:{interval:80,frames:[`);
  },
);

Deno.test(
  "import(url, { type: 'json' })",
  async () => {
    {
      const res = await fetch("http://localhost:8080/aleman@1.1.0/es2022/menu/menu.mjs");
      const text = await res.text();
      assert(res.ok, "should be found");
      assertEquals(res.headers.get("content-type"), "application/javascript; charset=utf-8");
      assertEquals(res.headers.get("cache-control"), "public, max-age=31536000, immutable");
      assertStringIncludes(text, `import("/aleman@1.1.0/menu/importmap.json?module")`);
    }
    {
      const res = await fetch(
        "http://localhost:8080/@uppy/dashboard@5.1.0/es2022/dashboard.mjs",
        { redirect: "follow" },
      );
      const js = await res.text();
      assert(res.ok, "should be found");
      assertEquals(res.headers.get("content-type"), "application/javascript; charset=utf-8");
      assertEquals(res.headers.get("cache-control"), "public, max-age=31536000, immutable");
      assertStringIncludes(js, '/@uppy/dashboard@5.1.0/package.json?module"', "Should contain package.json?module");
    }
  },
);

Deno.test(
  "json?module",
  async () => {
    const res = await fetch("http://localhost:8080/aleman@1.1.0/menu/importmap.json?module");
    const text = await res.text();
    assert(res.ok, "should be found");
    assertEquals(res.headers.get("content-type"), "application/javascript; charset=utf-8");
    assertEquals(res.headers.get("cache-control"), "public, max-age=31536000, immutable");
    const im = await import("http://localhost:8080/aleman@1.1.0/menu/importmap.json?module");
    assert(!!im.default.imports, "should have imports");
  },
);

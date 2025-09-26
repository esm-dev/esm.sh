import { assert, assertEquals } from "jsr:@std/assert";

// related issue: https://github.com/esm-dev/esm.sh/issues/1191
Deno.test(
  "import with { type: 'css' }",
  async () => {
    const res = await fetch("http://localhost:8080/aleman@1.0.7/es2022/menu/menu.mjs");
    const text = await res.text();
    assert(res.ok);
    assertEquals(res.headers.get("content-type"), "application/javascript; charset=utf-8");
    assertEquals(res.headers.get("cache-control"), "public, max-age=31536000, immutable");
    assert(text.includes(`import("/aleman@1.0.7/style.css?module")`));
  },
);

Deno.test(
  "css?module",
  async () => {
    const res = await fetch("http://localhost:8080/aleman@1.0.7/style.css?module");
    const text = await res.text();
    assert(res.ok);
    assertEquals(res.headers.get("content-type"), "application/javascript; charset=utf-8");
    assertEquals(res.headers.get("cache-control"), "public, max-age=31536000, immutable");
    assert(text.includes("const stylesheet = new CSSStyleSheet();"));
    assert(text.includes("stylesheet.replaceSync(`"));
    assert(text.includes("`);\nexport default stylesheet;"));
  },
);

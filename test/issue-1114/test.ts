import { assertEquals, assertFalse, assertStringIncludes } from "jsr:@std/assert";

for (const pkg of ["html-to-image@1.11.13", "safe-buffer@5.2.1"]) {
  Deno.test(`issue #1114 - ${pkg} standalone polyfills`, async () => {
    const res = await fetch(`http://localhost:8080/${pkg}?standalone&target=es2022`);
    assertEquals(res.status, 200);
    assertFalse((await res.text()).includes("/node/"));

    const bundle = await fetch(new URL(res.headers.get("x-esm-path")!, res.url));
    assertEquals(bundle.status, 200);
    const code = await bundle.text();
    assertStringIncludes(code, `/* esm.sh - ${pkg} */`);
    const mod = await import(`data:text/javascript;base64,${btoa(code)}`);
    if (pkg.startsWith("html-to-image")) {
      assertEquals(typeof mod.toPng, "function");
      assertEquals(typeof mod.toSvg, "function");
    } else {
      assertEquals(mod.Buffer.from("hello").toString("base64"), "aGVsbG8=");
    }
  });
}

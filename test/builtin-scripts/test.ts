import { assert, assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("builtin scripts", async () => {
  const { version: VERSION } = await fetch("http://localhost:8080/status.json").then((res) => res.json());

  {
    const res = await fetch("http://localhost:8080/x");
    assertEquals(res.headers.get("Etag"), `W/"${VERSION}"`);
    assertEquals(res.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertStringIncludes(res.headers.get("Vary") ?? "", "User-Agent");
    assertStringIncludes(await res.text(), "esm.sh/x");
  }

  {
    const res = await fetch("http://localhost:8080/run");
    assert(res.ok);
    assert(!res.redirected);
    assertEquals(res.headers.get("Etag"), `W/"${VERSION}"`);
    assertEquals(res.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertStringIncludes(res.headers.get("Vary") ?? "", "User-Agent");
    assertStringIncludes(await res.text(), "esm.sh/run");
  }
});

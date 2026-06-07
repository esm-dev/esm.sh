import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

// A types-only package whose compiled runtime JS is intentionally empty
// (only source-map comments, no import/export/CommonJS markers). The empty
// `.mjs` entry parses as `ExportsKind == ExportsNone`, which used to be
// misclassified as a "fake CommonJS module" and handed to cjs-module-lexer.
// The lexer then panicked resolving the `browser`-only `exports` subpath under
// `node`/`require` conditions, surfacing as an HTTP 500.
//
// related issue: https://github.com/esm-dev/esm.sh/issues/PENDING
Deno.test("empty .mjs module (types-only package) builds instead of 500", async () => {
  const res = await fetch(
    "http://localhost:8080/@solana/rpc-parsed-types@6.9.0?target=esnext",
    { headers: { "User-Agent": "i'm a browser" } },
  );
  const js = await res.text();
  assertEquals(res.status, 200);
  assertEquals(
    res.headers.get("content-type"),
    "application/javascript; charset=utf-8",
  );
  // The build should succeed and emit a (possibly empty) ES module rather than
  // panicking in cjs-module-lexer.
  assertStringIncludes(js, "/@solana/rpc-parsed-types@6.9.0/");
});

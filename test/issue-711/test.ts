import { assert, assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("issue #711", async () => {
  const res = await fetch("http://localhost:8080/@pyscript/core@0.1.5/core.mjs", {
    headers: {
      "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.3 Safari/605.1.15",
    },
  });
  await res.body?.cancel();
  const esmPath = res.headers.get("x-esm-path")!;
  assert(esmPath);
  assertStringIncludes(esmPath, "/es2022/");
  const res2 = await fetch("http://localhost:8080" + esmPath);
  res2.body?.cancel();
  assertEquals(res2.headers.get("content-type"), "application/javascript; charset=utf-8");
});

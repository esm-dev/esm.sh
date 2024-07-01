import { assert } from "jsr:@std/assert";

Deno.test("issue #732", async () => {
  const res = await fetch(
    "http://localhost:8080/lib0@0.2.83/webcrypto?target=es2022",
  );
  const id = res.headers.get("x-esm-path");
  res.body?.cancel();
  const res2 = await fetch(`http://localhost:8080/${id}`);
  const code = await res2.text();
  assert(!code.includes("crypto-browserify"));
});

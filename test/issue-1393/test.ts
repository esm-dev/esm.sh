import { assert } from "jsr:@std/assert";

// https://github.com/esm-dev/esm.sh/issues/1393
Deno.test("resolve nested package imports conditions", async () => {
  const res = await fetch(
    "http://localhost:8080/mcbe-leveldb-reader@5.0.1?target=es2022",
  );
  assert(res.ok);

  const code = await res.text();
  assert(!code.includes("/#nativeZlib"), code);
  assert(!code.includes("/node/zlib"), code);
});

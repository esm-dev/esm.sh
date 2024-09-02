import { assertEquals } from "jsr:@std/assert";

import { decodeBase64, encodeBase64 } from "https://esm.sh/jsr/@std/encoding@1.0.0/base64";

Deno.test("@std/encoding", async () => {
  assertEquals(encodeBase64("hello"), "aGVsbG8=");
  assertEquals(new TextDecoder().decode(decodeBase64("aGVsbG8=")), "hello");
});

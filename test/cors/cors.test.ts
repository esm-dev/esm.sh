import { assertEquals } from "jsr:@std/assert";

Deno.test("CORS", async () => {
  const res = await fetch("http://localhost:8080/react@18.2.0", {
    headers: {
      "Origin": "https://example.com",
    },
  });
  res.body?.cancel();
  assertEquals(res.headers.get("Access-Control-Allow-Origin"), "*");
  assertEquals(res.headers.get("Access-Control-Expose-Headers"), "X-Esm-Path, X-Typescript-Types");
});

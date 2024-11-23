import { assertEquals } from "jsr:@std/assert";

Deno.test("CORS", async () => {
  {
    const res = await fetch("http://localhost:8080/react@18.2.0", { method: "OPTIONS" });
    res.body?.cancel();
    assertEquals(res.status, 204);
    assertEquals(res.headers.get("Access-Control-Allow-Origin"), "*");
    assertEquals(res.headers.get("Access-Control-Allow-Headers"), "*");
    assertEquals(res.headers.get("Access-Control-Max-Age"), "86400");
  }
  {
    const res = await fetch("http://localhost:8080/react@18.2.0");
    res.body?.cancel();
    assertEquals(res.headers.get("Access-Control-Allow-Origin"), "*");
    assertEquals(res.headers.get("Access-Control-Expose-Headers"), "X-ESM-Path, X-TypeScript-Types");
    assertEquals(res.headers.get("Vary"), "User-Agent");
  }
  {
    const res = await fetch("http://localhost:8080/react@18.2.0?no-dts");
    res.body?.cancel();
    assertEquals(res.headers.get("Access-Control-Allow-Origin"), "*");
    assertEquals(res.headers.get("Access-Control-Expose-Headers"), "X-ESM-Path");
    assertEquals(res.headers.get("Vary"), "User-Agent");
  }
  {
    const res = await fetch("http://localhost:8080/react@18.2.0", {
      headers: {
        "Origin": "https://example.com",
      },
    });
    res.body?.cancel();
    assertEquals(res.headers.get("Access-Control-Allow-Origin"), "*");
    assertEquals(res.headers.get("Access-Control-Expose-Headers"), "X-ESM-Path, X-TypeScript-Types");
    assertEquals(res.headers.get("Vary"), "User-Agent");
  }
});

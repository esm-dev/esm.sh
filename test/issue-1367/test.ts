import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("issue #1367", async () => {
  {
    const res = await fetch(
      "http://localhost:8080/@graphiql/react@0.37.5?standalone",
      { headers: { "User-Agent": "i'm a browser" } },
    );
    const js = await res.text();
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("content-type"), "application/javascript; charset=utf-8");
  }
  {
    const res = await fetch(
      "http://localhost:8080/@graphiql/react@0.37.5?standalone&meta",
      { headers: { "User-Agent": "i'm a browser" } },
    );
    const meta = await res.json();
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("content-type"), "application/json; charset=utf-8");
    assertEquals(meta.peerImports.length, 5);
  }
  {
    const res = await fetch(
      "http://localhost:8080/@graphiql/react@0.37.5?standalone&external=react,react-dom&meta",
      { headers: { "User-Agent": "i'm a browser" } },
    );
    const meta = await res.json();
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("content-type"), "application/json; charset=utf-8");
    assertEquals(meta.peerImports.length, 1);
  }
  {
    const res = await fetch(
      "http://localhost:8080/@graphiql/react@0.37.5?standalone&external=*&meta",
      { headers: { "User-Agent": "i'm a browser" } },
    );
    const meta = await res.json();
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("content-type"), "application/json; charset=utf-8");
    assertEquals(meta.peerImports, undefined);
  }
});

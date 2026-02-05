import { assert, assertEquals } from "jsr:@std/assert";

Deno.test("?meta query", async () => {
  {
    const res = await fetch("http://localhost:8080/react@19?meta", { headers: { "User-Agent": "i'm a browser" } });
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("cache-control"), "public, max-age=600");
    assertEquals(res.headers.get("content-type"), "application/json; charset=utf-8");
    const meta = await res.json();
    assertEquals(meta.name, "react");
    assert(meta.version.startsWith("19."));
    assert(meta.exports.includes("./jsx-runtime"));
    assert(!meta.imports);
    assert(meta.integrity.startsWith("sha384-"));
  }
  {
    const res = await fetch("http://localhost:8080/react-dom@19.2.3?meta", { headers: { "User-Agent": "i'm a browser" } });
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("cache-control"), "public, max-age=31536000, immutable");
    assertEquals(res.headers.get("content-type"), "application/json; charset=utf-8");
    const meta = await res.json();
    assertEquals(meta.name, "react-dom");
    assertEquals(meta.version, "19.2.3");

    assert(meta.exports.includes("./client"));
    assert(meta.exports.includes("./server"));
    assert(!meta.imports);
    assertEquals(meta.peerImports?.length, 1);
    assert(meta.peerImports?.[0].startsWith("/react@19.2."));
  }
  {
    const res = await fetch("http://localhost:8080/react-dom@19.2.3/client?meta", { headers: { "User-Agent": "i'm a browser" } });
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("cache-control"), "public, max-age=31536000, immutable");
    assertEquals(res.headers.get("content-type"), "application/json; charset=utf-8");
    const meta = await res.json();
    assertEquals(meta.name, "react-dom");
    assertEquals(meta.version, "19.2.3");
    assertEquals(meta.subpath, "client");

    assert(!meta.exports);
    assertEquals(meta.imports?.length, 2);
    assert(meta.imports?.[0].startsWith("/react-dom@19.2.3/"));
    assert(meta.imports?.[1].startsWith("/scheduler@^0.27.0?"));
    assertEquals(meta.peerImports?.length, 1);
    assert(meta.peerImports?.[0].startsWith("/react@19.2."));
  }
});

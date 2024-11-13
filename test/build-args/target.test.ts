import { assert, assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("target from ua", async () => {
  {
    const res = await fetch("http://localhost:8080/react@18.3.1");
    assertEquals(res.status, 200);
    assertStringIncludes(res.headers.get("Vary")!, "User-Agent");
    assertStringIncludes(await res.text(), "/denonext/");
  }
  {
    const res = await fetch("http://localhost:8080/react@18.3.1", { headers: { "User-Agent": "Deno/1.33.2" } });
    assertEquals(res.status, 200);
    assertStringIncludes(res.headers.get("Vary")!, "User-Agent");
    assertStringIncludes(await res.text(), "/denonext/");
  }
  {
    const res = await fetch("http://localhost:8080/react@18.3.1", { headers: { "User-Agent": "Deno/1.33.1" } });
    assertEquals(res.status, 200);
    assertStringIncludes(res.headers.get("Vary")!, "User-Agent");
    assertStringIncludes(await res.text(), "/deno/");
  }
  {
    const res = await fetch("http://localhost:8080/react@18.3.1", { headers: { "User-Agent": "Node.js/22.0.0" } });
    assertEquals(res.status, 200);
    assertStringIncludes(res.headers.get("Vary")!, "User-Agent");
    assertStringIncludes(await res.text(), "/node/");
  }
  {
    const res = await fetch("http://localhost:8080/react@18.3.1", { headers: { "User-Agent": "ES/2024" } });
    assertEquals(res.status, 200);
    assertStringIncludes(res.headers.get("Vary")!, "User-Agent");
    assertStringIncludes(await res.text(), "/es2024/");
  }
  {
    const res = await fetch("http://localhost:8080/react@18.3.1", { headers: { "User-Agent": "whatever" } });
    assertEquals(res.status, 200);
    assertStringIncludes(res.headers.get("Vary")!, "User-Agent");
    assertStringIncludes(await res.text(), "/es2022/");
  }
});

Deno.test("target from query", async () => {
  {
    const res = await fetch("http://localhost:8080/react@18.3.1?target=denonext");
    assertEquals(res.status, 200);
    assert(!res.headers.get("Vary")!.includes("User-Agent"));
    assertStringIncludes(await res.text(), "/denonext/");
  }
  {
    const res = await fetch("http://localhost:8080/react@18.3.1?target=deno");
    assertEquals(res.status, 200);
    assert(!res.headers.get("Vary")!.includes("User-Agent"));
    assertStringIncludes(await res.text(), "/deno/");
  }
  {
    const res = await fetch("http://localhost:8080/react@18.3.1?target=node");
    assertEquals(res.status, 200);
    assert(!res.headers.get("Vary")!.includes("User-Agent"));
    assertStringIncludes(await res.text(), "/node/");
  }
  {
    const res = await fetch("http://localhost:8080/react@18.3.1?target=es2024");
    assertEquals(res.status, 200);
    assert(!res.headers.get("Vary")!.includes("User-Agent"));
    assertStringIncludes(await res.text(), "/es2024/");
  }
});

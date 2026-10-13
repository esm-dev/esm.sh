import { assert, assertEquals, assertStringIncludes } from "jsr:@std/assert";
import { runInNewContext } from "node:vm";

Deno.test("builtin scripts", async (t) => {
  const { version: VERSION } = await fetch("http://localhost:8080/status.json").then((res) => res.json());

  await t.step("tsx", async () => {
    const res = await fetch("http://localhost:8080/tsx");
    assert(res.ok);
    assert(!res.redirected);
    assertEquals(res.headers.get("Etag"), `W/"${VERSION}"`);
    assertEquals(res.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertStringIncludes(res.headers.get("Vary") ?? "", "User-Agent");
    assertStringIncludes(await res.text(), "esm.sh/tsx");
  });

  await t.step("run", async () => {
    const res = await fetch("http://localhost:8080/run");
    assert(res.ok);
    assert(!res.redirected);
    assertEquals(res.headers.get("Etag"), `W/"${VERSION}"`);
    assertEquals(res.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertStringIncludes(res.headers.get("Vary") ?? "", "User-Agent");
    assertStringIncludes(await res.text(), "esm.sh/tsx");
  });
});

Deno.test("tsx transforms scripts when localStorage access is denied", async () => {
  const url = "http://localhost:8080/tsx";
  const source = await fetch(url).then((res) => res.text());
  const code = "export const value: number = 42;";
  const requests: string[] = [];
  const script = await new Promise<{ type: string; textContent: string }>((resolve) => {
    runInNewContext(source.replaceAll("import.meta.url", JSON.stringify(url)), {
      document: {
        querySelectorAll: () => [{ type: "text/ts", textContent: code, replaceWith: resolve }],
        createElement: () => ({}),
      },
      location: { hostname: "example.com" },
      get localStorage() {
        throw new DOMException("Access is denied for this document", "SecurityError");
      },
      crypto,
      TextEncoder,
      TextDecoder,
      URL,
      fetch: (url: URL, options?: RequestInit) => {
        requests.push(options?.method ?? "GET");
        if (!options) {
          assert(url.pathname.startsWith("/+"));
          return Promise.resolve(new Response(null, { status: 404 }));
        }
        assertEquals(url.pathname, "/transform");
        assertEquals(JSON.parse(options.body as string).code, code);
        return fetch(url, options);
      },
    });
  });
  assertEquals(requests, ["GET", "POST"]);
  assertEquals(script.type, "module");
  const mod = await import("data:application/javascript," + encodeURIComponent(script.textContent));
  assertEquals(mod.value, 42);
});

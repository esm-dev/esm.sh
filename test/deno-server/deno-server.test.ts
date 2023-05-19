import {
  assertEquals,
  assertStringIncludes,
} from "https://deno.land/std@0.180.0/testing/asserts.ts";

import { FileStorage, init, serve } from "http://localhost:8080/server";

init({
  ESM_SERVER_ORIGIN: "http://localhost:8080",
});

Deno.test("deno server", async () => {
  const fs = new FileStorage();
  const res = await fetch(
    "http://localhost:8080/stable/react@18.2.0/deno/react.mjs",
  );
  const url = new URL(res.url);
  await fs.put(url.pathname, res.body!, {
    httpMetadata: { contentType: res.headers.get("content-type")! },
  });
  const ret = await fs.get(url.pathname);
  assertEquals(ret!.httpMetadata, {
    contentType: res.headers.get("content-type")!,
  });
  assertStringIncludes(await new Response(ret!.body).text(), "createElement");

  const ac = new AbortController();
  await serve((req, { url }) => {
    if (url.pathname === "/") {
      return new Response("<h1>Welcome to use esm.sh served by Deno.</h1>", {
        headers: { "Content-Type": "text/html" },
      });
    }
  }, {
    signal: ac.signal,
    port: 8787,
    onListen: async ({ port }) => {
      assertEquals(port, 8787);
      let res = await fetch("http://localhost:8787/", {
        headers: { "User-Agent": "Chrome/90.0.4430.212" },
      });
      assertEquals(res.status, 200);
      assertEquals(res.headers.get("content-type"), "text/html");
      assertEquals(
        await res.text(),
        "<h1>Welcome to use esm.sh served by Deno.</h1>",
      );
      ac.abort();
    },
  });
});

import { dirname, join } from "https://deno.land/std@0.220.0/path/mod.ts";
import { assert, assertEquals, assertStringIncludes } from "jsr:@std/assert";

const testRegisterToken = "1E372D421838559CE40E4CF955B3A40E30EEB7AA";
const env = {
  ESM_SERVER_ORIGIN: "http://localhost:8080",
  NPMRC: `{ "registries": { "@private": { "registry": "http://localhost:8082/", "token": "${testRegisterToken}" }}}`,
};
const workerOrigin = "http://localhost:8081";
const ac = new AbortController();
const closeServer = () => ac.abort();

const cache = {
  _store: new Map<string, Response>(),
  match(req: URL) {
    return Promise.resolve(cache._store.get(req.href)?.clone() || null);
  },
  put(req: URL, res: Response) {
    cache._store.set(req.href, res);
    return Promise.resolve();
  },
};

const R2 = {
  _store: new Map(),
  async get(key: string): Promise<
    {
      body: ReadableStream<Uint8Array>;
      httpMetadata?: any;
      customMetadata?: Record<string, string>;
    } | null
  > {
    const ret = this._store.get(key);
    if (ret) {
      return { ...ret, body: new Response(ret.value).body! };
    }
    return null;
  },
  async put(
    key: string,
    value: ArrayBufferLike | ArrayBuffer | ReadableStream,
    options?: any,
  ): Promise<void> {
    this._store.set(key, { value: await new Response(value).arrayBuffer(), ...options });
  },
  async delete(key: string | string[]): Promise<void> {
    if (Array.isArray(key)) {
      for (const k of key) {
        this._store.delete(k);
      }
    } else {
      this._store.delete(key);
    }
  },
};

const LEGACY_WORKER = {
  fetch: (req: Request) => {
    return new Response(req.url);
  },
};

// build esm worker
await run("pnpm", "i");
await run("node", "build.mjs");

const { withESMWorker, version } = await import("../../worker/dist/index.js#" + Date.now().toString(36));
const worker = withESMWorker((_req: Request, _env: typeof env, ctx: { url: URL }) => {
  if (ctx.url.pathname === "/") {
    return new Response("<h1>Welcome to esm.sh!</h1>", {
      headers: { "content-type": "text/html" },
    });
  }
}, cache);

// start the worker
Deno.serve(
  { port: 8081, signal: ac.signal },
  (req) => worker.fetch(req, { ...env, R2, LEGACY_WORKER }, { waitUntil: () => {} }),
);

// start the private registry
Deno.serve(
  { port: 8082, signal: ac.signal },
  (req) => {
    const auth = req.headers.get("authorization");
    if (auth !== "Bearer " + testRegisterToken) {
      return new Response("unauthorized", { status: 401 });
    }

    const url = new URL(req.url);
    const pathname = decodeURIComponent(url.pathname);

    if (pathname === "/@private/pkg/1.0.0.tgz") {
      try {
        const buf = Deno.readFileSync(join(dirname(new URL(import.meta.url).pathname), "pkg-1.0.0.tgz"));
        return new Response(buf, {
          headers: {
            "content-type": "application/octet-stream",
            "content-length": buf.byteLength.toString(),
          },
        });
      } catch (error) {
        console.error(error);
        return new Response(error.message, { status: 500 });
      }
    }

    if (pathname === "/@private/pkg") {
      return Response.json({
        "name": "@private/pkg",
        "description": "My private package",
        "dist-tags": {
          "latest": "1.0.0",
        },
        "versions": {
          "1.0.0": {
            "name": "@private/pkg",
            "description": "My private package",
            "version": "1.0.0",
            "type": "module",
            "module": "dist/index.js",
            "types": "dist/index.d.ts",
            "files": [
              "dist/",
            ],
            "dist": {
              "tarball": "http://localhost:8082/@private/pkg/1.0.0.tgz",
              // shasum -a 1 pkg-1.0.0.tgz
              "shasum": "71080422342aac4549dca324bf4361596288ba17",
              // openssl dgst -binary -sha512 pkg-1.0.0.tgz | openssl base64
              "integrity": "sha512-sYRCpe+Q0gh6RfBhHsUveq3ihSADt64X8Ag7DCpAlcKrwI/wUF4yrEYlzb9eEJO0t/89Lb+ZSmG7qU4DMsBkrg==",
            },
          },
        },
        "time": {
          "created": "2024-06-14T00:00:00.000Z",
          "modified": "2024-06-14T00:00:00.000Z",
          "1.0.0": "2024-06-14T00:00:00.000Z",
        },
      });
    }

    return new Response("not found", { status: 404 });
  },
);

Deno.test("esm-worker", { sanitizeOps: false, sanitizeResources: false }, async (t) => {
  // wait for the server to start
  await new Promise((resolve) => setTimeout(resolve, 100));

  await t.step("bad url", async () => {
    const res = await fetch(`${workerOrigin}/.git/HEAD`);
    res.body?.cancel();
    assertEquals(res.status, 404);

    const res2 = await fetch(`${workerOrigin}/wp-admin/index.php`);
    res2.body?.cancel();
    assertEquals(res2.status, 404);

    const res3 = await fetch(`${workerOrigin}//react@18`);
    res3.body?.cancel();
    assertEquals(res3.status, 400);

    const res4 = await fetch(`${workerOrigin}/react>dom@18`);
    res4.body?.cancel();
    assertEquals(res4.status, 400);

    const res5 = await fetch(`${workerOrigin}/react@17.17.17`);
    res5.body?.cancel();
    assertEquals(res5.status, 404);
  });

  await t.step("custom homepage", async () => {
    const res = await fetch(workerOrigin, {
      headers: { "User-Agent": "Chrome/90.0.4430.212" },
    });
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("Content-Type"), "text/html");
    const text = await res.text();
    assertStringIncludes(text, "<h1>Welcome to esm.sh!</h1>");
  });

  await t.step("status.json", async () => {
    const res = await fetch(`${workerOrigin}/status.json`);
    const ret = await res.json();
    assertEquals(typeof ret.version, "number");
  });

  await t.step("embed scripts/polyfills/types", async () => {
    const res2 = await fetch(`${workerOrigin}/run.d.ts`);
    assertEquals(res2.status, 200);
    assertEquals(res2.headers.get("Content-Type"), "application/typescript; charset=utf-8");
    assertEquals(res2.headers.get("Etag"), `W/"${version}"`);
    assertEquals(res2.headers.get("Cache-Control"), "public, max-age=86400");
    assertStringIncludes(await res2.text(), "export interface RunOptions");

    const res3 = await fetch(`${workerOrigin}/node/process.js`);
    assertEquals(res3.status, 200);
    assertEquals(res3.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertEquals(res3.headers.get("Etag"), `W/"${version}"`);
    assertEquals(res3.headers.get("Cache-Control"), "public, max-age=86400");
    assertStringIncludes(res3.headers.get("Vary")!, "User-Agent");
    assertStringIncludes(await res3.text(), "nextTick");

    const res4 = await fetch(`${workerOrigin}/node/process.js`, { headers: { "If-None-Match": `W/"${version}"` } });
    res4.body?.cancel();
    assertEquals(res4.status, 304);

    const res5 = await fetch(`${workerOrigin}/node/fs.js`);
    assertEquals(res5.status, 200);
    assertEquals(res5.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    const js = await res5.text();
    const m = js.match(/\.\/chunk-[a-f0-9]+\.js/);
    assert(m);

    const res6 = await fetch(`${workerOrigin}/node${m![0].slice(1)}`);
    res6.body?.cancel();
    assertEquals(res6.status, 200);
    assertEquals(res6.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertEquals(res6.headers.get("Cache-Control"), "public, max-age=31536000, immutable");

    const fs = await import(`${workerOrigin}/node/fs.js`);
    fs.writeFileSync("foo.txt", "bar", "utf8");
    assertEquals(fs.readFileSync("foo.txt", "utf8"), "bar");
  });

  await t.step("npm modules", async () => {
    const res = await fetch(`${workerOrigin}/react`, { headers: { "User-Agent": "ES/2022" } });
    const modUrl = new URL(res.headers.get("x-esm-path")!, workerOrigin);
    assert(modUrl.pathname.endsWith("/es2022/react.mjs"));
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=600");
    assertStringIncludes(res.headers.get("X-Typescript-Types")!, "/@types/react@");
    assertStringIncludes(await res.text(), modUrl.pathname);

    const res3 = await fetch(modUrl);
    assertEquals(res3.status, 200);
    assertEquals(res3.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertEquals(res3.headers.get("Cache-Control"), "public, max-age=31536000, immutable");
    assertStringIncludes(await res3.text(), "createElement");

    const dtsUrl = res.headers.get("X-Typescript-Types")!;
    assert(dtsUrl.startsWith(workerOrigin));
    assert(dtsUrl.endsWith(".d.ts"));

    const res4 = await fetch(dtsUrl);
    res4.body?.cancel();
    assertEquals(res4.status, 200);
    assertEquals(res4.headers.get("Content-Type"), "application/typescript; charset=utf-8");

    const res5 = await fetch(`${workerOrigin}/react@^18.2.0`, { headers: { "User-Agent": "ES/2022" } });
    assertEquals(res5.status, 200);
    assertEquals(res5.headers.get("Cache-Control"), "public, max-age=600");
    assertStringIncludes(await res5.text(), `"/react@18.`);

    const res6 = await fetch(`${workerOrigin}/react@17.0.2`);
    res6.body?.cancel();
    assertEquals(res6.status, 200);
    assertEquals(res6.headers.get("Cache-Control"), "public, max-age=31536000, immutable");
    const modUrl2 = new URL(res6.headers.get("x-esm-path")!, workerOrigin);
    const res7 = await fetch(modUrl2);
    assertEquals(res7.status, 200);
    // inline "object.assign" polyfill
    assertStringIncludes(await res7.text(), `from "data:text/javascript;base64,ZXhwb3J0IGRlZmF1bHQgT2JqZWN0LmFzc2lnbg=="`);

    const res8 = await fetch(`${workerOrigin}/react-dom@18.2.0?external=react`);
    res8.body?.cancel();
    assertEquals(res8.status, 200);
    assertEquals(res8.headers.get("Cache-Control"), "public, max-age=31536000, immutable");
    const modUrl3 = new URL(res8.headers.get("x-esm-path")!, workerOrigin);
    const res9 = await fetch(modUrl3);
    assertEquals(res9.status, 200);
    assertStringIncludes(await res9.text(), `from "react"`);

    const res10 = await fetch(`${workerOrigin}/typescript@5.5.4/es2022/typescript.mjs`);
    const js = await res10.text();
    assertEquals(res10.status, 200);
    assertEquals(res10.headers.get("Cache-Control"), "public, max-age=31536000, immutable");
    assertStringIncludes(js, `__Process$`);
    assert(!js.includes("/node/process.js"));

    const res11 = await fetch(`${workerOrigin}/typescript@5.5.4/es2022/typescript.mjs.map`);
    assertEquals(res11.status, 200);
    assertEquals(res11.headers.get("Cache-Control"), "public, max-age=31536000, immutable");
    assertEquals(res11.headers.get("Content-Type"), "application/json; charset=utf-8");
  });

  await t.step("npm modules (submodule)", async () => {
    const res = await fetch(`${workerOrigin}/react-dom@18/server`, {
      headers: { "User-Agent": "ES/2022" },
    });
    const modUrl = new URL(res.headers.get("x-esm-path")!, workerOrigin);
    assert(modUrl.pathname.endsWith("/es2022/server.js"));
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=600");
    assertStringIncludes(res.headers.get("X-Typescript-Types")!, "/@types/react-dom@");
    assertStringIncludes(await res.text(), modUrl.pathname);

    const res3 = await fetch(modUrl);
    assertEquals(res3.status, 200);
    assertEquals(res3.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertEquals(res3.headers.get("Cache-Control"), "public, max-age=31536000, immutable");
    assertStringIncludes(await res3.text(), "renderToString");

    const res4 = await fetch(new URL(modUrl.pathname + ".map", modUrl));
    assertEquals(res4.status, 200);
    assertEquals(res4.headers.get("Content-Type"), "application/json; charset=utf-8");
  });

  await t.step("npm assets", async () => {
    const res = await fetch(`${workerOrigin}/react/package.json`, { redirect: "manual" });
    res.body?.cancel();
    assertEquals(res.status, 302);
    assert(res.headers.get("Location")!.startsWith(`${workerOrigin}/react@`));
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=600");
    const res2 = await fetch(res.headers.get("Location")!);

    assertEquals(res2.status, 200);
    assertEquals(res2.headers.get("Content-Type"), "application/json");
    assertEquals(res2.headers.get("Cache-Control"), "public, max-age=31536000, immutable");
    const pkgJson = await res2.json();
    assertEquals(pkgJson.name, "react");
  });

  await t.step("npm assets (raw)", async () => {
    const res = await fetch(`${workerOrigin}/playground-elements@0.18.1/playground-service-worker.js?raw`);
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=31536000, immutable");
    assertStringIncludes(await res.text(), "!function(){");

    const res2 = await fetch(`${workerOrigin}/playground-elements@0.18.1&raw/playground-service-worker.js`);
    assertEquals(res2.status, 200);
    assertEquals(res2.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertEquals(res2.headers.get("Cache-Control"), "public, max-age=31536000, immutable");
    assertStringIncludes(await res2.text(), "!function(){");
  });

  await t.step("gh modules", async () => {
    const res = await fetch(`${workerOrigin}/gh/microsoft/tslib`, { headers: { "User-Agent": "ES/2022" } });
    const modUrl = new URL(res.headers.get("x-esm-path")!, workerOrigin);
    assert(modUrl.pathname.endsWith("/es2022/tslib.mjs"));
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=600");
    assertStringIncludes(res.headers.get("X-Typescript-Types")!, "/gh/microsoft/tslib@");
    assertStringIncludes(await res.text(), modUrl.pathname);

    const res3 = await fetch(modUrl);
    assertEquals(res3.status, 200);
    assertEquals(res3.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertEquals(res3.headers.get("Cache-Control"), "public, max-age=31536000, immutable");
    assertStringIncludes(await res3.text(), "export{");
  });

  await t.step("gh assets", async () => {
    const res = await fetch(
      `${workerOrigin}/gh/microsoft/fluentui-emoji/assets/Alien/Flat/alien_flat.svg`,
      { redirect: "manual" },
    );
    res.body?.cancel();
    assertEquals(res.status, 302);
    const redirectTo = res.headers.get("Location")!;
    assert(redirectTo.startsWith(`${workerOrigin}/gh/microsoft/fluentui-emoji@`));
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=600");

    const res2 = await fetch(redirectTo);
    const svg = await res2.text();
    assertEquals(res2.status, 200);
    assertEquals(res2.headers.get("Content-Type"), "image/svg+xml");
    assertEquals(res2.headers.get("Cache-Control"), "public, max-age=31536000, immutable");
    assertStringIncludes(svg, "<svg");
  });

  await t.step("jsr", async () => {
    const res = await fetch(
      `${workerOrigin}/jsr/@std/encoding/base64`,
      { headers: { "User-Agent": "ES/2022" } },
    );
    const modUrl = new URL(res.headers.get("x-esm-path")!, workerOrigin);
    assert(modUrl.pathname.endsWith("/es2022/base64.js"));
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=600");
    assertStringIncludes(res.headers.get("X-Typescript-Types")!, "/@jsr/std__encoding@");
    assertStringIncludes(await res.text(), modUrl.pathname);

    const { encodeBase64, decodeBase64 } = await import(modUrl.href);
    assertEquals(encodeBase64("hello"), "aGVsbG8=");
    assertEquals(new TextDecoder().decode(decodeBase64("aGVsbG8=")), "hello");
  });

  await t.step("builtin scripts", async () => {
    const res = await fetch(`${workerOrigin}/run`, { redirect: "manual" });
    assert(res.ok);
    assert(!res.redirected);
    assertEquals(res.headers.get("Etag"), `W/"${version}"`);
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=86400");
    assertEquals(res.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertStringIncludes(res.headers.get("Vary") ?? "", "User-Agent");
    assertStringIncludes(res.headers.get("Vary") ?? "", "Referer");
    assertStringIncludes(await res.text(), '("/transform")');

    const dtsUrl = res.headers.get("X-Typescript-Types")!;
    assert(dtsUrl.startsWith(workerOrigin));
    assert(dtsUrl.endsWith(".d.ts"));

    const res2 = await fetch(`${workerOrigin}/run?target=es2022`, { headers: { "referer": "http://localhost:8080/sw.js" } });
    const code = await res2.text();
    assert(!res2.headers.get("Vary")?.includes("User-Agent"));
    assertStringIncludes(res.headers.get("Vary") ?? "", "Referer");
    assertStringIncludes(code, 'from"/esm-compiler@');
    assertStringIncludes(code, '/es2022/esm-compiler.mjs"');

    const res4 = await fetch(`${workerOrigin}/tsx`);
    assertEquals(res4.headers.get("Etag"), `W/"${version}"`);
    assertEquals(res4.headers.get("Cache-Control"), "public, max-age=86400");
    assertEquals(res4.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertStringIncludes(res4.headers.get("Vary") ?? "", "User-Agent");
    assertStringIncludes(await res4.text(), "esm.sh/tsx");
  });

  await t.step("transform api", async () => {
    const options = {
      code: `
        import { renderToString } from "preact-render-to-string";
        export default () => renderToString(<h1>Hello world!</h1>);
      `,
      filename: "source.jsx",
      target: "es2022",
      importMap: {
        imports: {
          "@jsxImportSource": "https://preact@10.13.2",
          "preact-render-to-string": "https://esm.sh/preact-render-to-string6.0.2",
        },
      },
      sourceMap: true,
    };
    const hash = await computeHash("jsx" + options.code + JSON.stringify(options.importMap) + options.target + options.sourceMap);
    const res1 = await fetch(`${workerOrigin}/transform`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(options),
    });
    const ret = await res1.json();
    assertStringIncludes(ret.code, `"https://preact@10.13.2/jsx-runtime"`);
    assertStringIncludes(ret.code, `"https://esm.sh/preact-render-to-string6.0.2"`);
    assertStringIncludes(ret.map, `"mappings":`);

    const res2 = await fetch(`${workerOrigin}/+${hash}.mjs`, {
      headers: { "User-Agent": "Chrome/90.0.4430.212" },
    });
    assertEquals(res2.status, 200);
    assertEquals(res2.headers.get("Content-Type"), "application/javascript; charset=utf-8");

    const js = await res2.text();
    assertEquals(js, ret.code);

    const res3 = await fetch(`${workerOrigin}/+${hash}.mjs.map`, {
      headers: { "User-Agent": "Chrome/90.0.4430.212" },
    });
    assertEquals(res3.status, 200);
    assertEquals(res3.headers.get("Content-Type"), "application/json; charset=utf-8");

    const map = await res3.text();
    assertEquals(map, ret.map);
  });

  await t.step("purge api", async () => {
    const fd = new FormData();
    fd.append("package", "react");
    fd.append("version", "18");
    const res = await fetch(`${workerOrigin}/purge`, {
      method: "POST",
      body: fd,
    });
    assertEquals(res.status, 200);
    const ret: any = await res.json();
    assert(Array.isArray(ret.deletedPkgs));
    assert(Array.isArray(ret.deletedFiles));
    assert(ret.deletedPkgs.length > 0);
    assert(ret.deletedFiles.length > 0);
  });

  await t.step("module with different UAs", async () => {
    const fetchModule = async (ua: string) => {
      const res = await fetch(`${workerOrigin}/react@18.2.0`, {
        headers: { "User-Agent": ua },
      });
      res.body?.cancel();
      assertStringIncludes(res.headers.get("Vary") ?? "", "User-Agent");
      return res.headers.get("x-esm-path")!;
    };

    assertStringIncludes(
      await fetchModule(
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/90.0.0.0 Safari/537.36",
      ),
      "/es2021/",
    );
    assertStringIncludes(
      await fetchModule(
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.0.0 Safari/537.36",
      ),
      "/es2023/",
    );
    assertStringIncludes(
      await fetchModule(
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Safari/537.36",
      ),
      "/es2023/",
    );
    assertStringIncludes(await fetchModule("HeadlessChrome/109"), "/es2023/");
    assertStringIncludes(
      await fetchModule(
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.3 Safari/605.1.15",
      ),
      "/es2021/",
    );
    assertStringIncludes(
      await fetchModule(
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.6 Safari/605.1.1",
      ),
      "/es2023/",
    );
    assertStringIncludes(await fetchModule("Deno/1.33.1"), "/deno/");
    assertStringIncludes(await fetchModule("Deno/1.33.2"), "/denonext/");
    assertStringIncludes(await fetchModule("ES/2022"), "/es2022/");
  });

  await t.step("CORS", async () => {
    const res = await fetch(`${workerOrigin}/react@18.2.0`, {
      headers: {
        "Origin": "https://example.com",
      },
    });
    res.body?.cancel();
    assertEquals(res.headers.get("Access-Control-Allow-Origin"), "*");
    assertEquals(res.headers.get("Access-Control-Expose-Headers"), "X-Esm-Path, X-TypeScript-Types");
  });

  await t.step("fix urls", async () => {
    {
      const res = await fetch(`${workerOrigin}/react`, { redirect: "manual" });
      res.body?.cancel();
      assertEquals(res.status, 302);
      assertEquals(res.headers.get("cache-control"), "public, max-age=600");
      assertEquals(res.headers.get("Vary"), "User-Agent");
      assertStringIncludes(res.headers.get("location")!, `${workerOrigin}/react@`);
    }
    {
      const res = await fetch(`${workerOrigin}/react@18`, { redirect: "manual" });
      res.body?.cancel();
      assertEquals(res.status, 302);
      assertEquals(res.headers.get("cache-control"), "public, max-age=600");
      assertEquals(res.headers.get("Vary"), "User-Agent");
      assertStringIncludes(res.headers.get("location")!, `${workerOrigin}/react@18.`);
    }
    {
      const res = await fetch(`${workerOrigin}/react@18/es2022/react.mjs`, { redirect: "manual" });
      res.body?.cancel();
      assertEquals(res.status, 302);
      assertEquals(res.headers.get("cache-control"), "public, max-age=600");
      assertStringIncludes(res.headers.get("location")!, `${workerOrigin}/react@18.`);
      assertStringIncludes(res.headers.get("location")!, "/es2022/react.mjs");
    }
    "`/#/` in pathname";
    {
      const res = await fetch(`${workerOrigin}/es5-ext@^0.10.50/string/%23/contains?target=denonext`, { redirect: "manual" });
      res.body?.cancel();
      assertEquals(res.status, 302);
      assertEquals(res.headers.get("cache-control"), "public, max-age=600");
      assertStringIncludes(res.headers.get("location")!, `${workerOrigin}/es5-ext@0.10.`);
      assertStringIncludes(res.headers.get("location")!, "/string/%23/contains");
    }
    {
      const res = await fetch(
        `${workerOrigin}/lightningcss-wasm@1.19.0/es2022/lightningcss_node.wasm`,
        { redirect: "manual" },
      );
      res.body?.cancel();
      assertEquals(res.status, 301);
      assertEquals(res.headers.get("location"), `${workerOrigin}/lightningcss-wasm@1.19.0/lightningcss_node.wasm`);
    }
  });

  await t.step("private registry", async () => {
    const res0 = await fetch(`http://localhost:8082/@private/pkg`);
    res0.body?.cancel();
    assertEquals(res0.status, 401);

    const res1 = await fetch(`http://localhost:8082/@private/pkg`, {
      headers: { authorization: "Bearer " + testRegisterToken },
    });
    assertEquals(res1.status, 200);
    const pkg = await res1.json();
    assertEquals(pkg.name, "@private/pkg");

    const res2 = await fetch(`http://localhost:8082/@private/pkg/1.0.0.tgz`);
    res2.body?.cancel();
    assertEquals(res2.status, 401);

    const res3 = await fetch(`http://localhost:8082/@private/pkg/1.0.0.tgz`, {
      headers: { authorization: "Bearer " + testRegisterToken },
    });
    res3.body?.cancel();
    assertEquals(res3.status, 200);

    const { key } = await import(`${workerOrigin}/@private/pkg`);
    assertEquals(key, "secret");
  });

  await t.step("fallback to legacy worker", async () => {
    const res = await fetch(`${workerOrigin}/stable/react`);
    assertEquals(await res.text(), `${workerOrigin}/stable/react`);

    const res2 = await fetch(`${workerOrigin}/v135/react-dom`);
    assertEquals(await res2.text(), `${workerOrigin}/v135/react-dom`);

    const res3 = await fetch(`${workerOrigin}/react-dom?pin=v135`);
    assertEquals(await res3.text(), `${workerOrigin}/react-dom?pin=v135`);

    const res4 = await fetch(`${workerOrigin}/build`);
    assertEquals(await res4.text(), `${workerOrigin}/build`);

    const res5 = await fetch(`${workerOrigin}/~41f4075e7fabb79f155504bd2d73c678b218111f`);
    assertEquals(await res5.text(), `${workerOrigin}/~41f4075e7fabb79f155504bd2d73c678b218111f`);
  });

  console.log("storage summary:");
  console.log("Cache", [...cache._store.keys()].map((url) => `${url} (${cache._store.get(url)!.headers.get("Cache-Control")})`));
  console.log("R2", [...R2._store.keys()]);

  closeServer();
});

async function computeHash(input: string): Promise<string> {
  const buffer = new Uint8Array(
    await crypto.subtle.digest(
      "SHA-1",
      new TextEncoder().encode(input),
    ),
  );
  return [...buffer].map((b) => b.toString(16).padStart(2, "0")).join("");
}

async function run(name: string, ...args: string[]) {
  const cwd = join(new URL(import.meta.url).pathname, "../../../worker");
  const command = new Deno.Command(name, {
    args,
    stdin: "inherit",
    stdout: "inherit",
    cwd,
  });
  const status = await command.spawn().status;
  if (!status.success) {
    throw new Error(`Failed to run ${name} ${args.join(" ")}`);
  }
}

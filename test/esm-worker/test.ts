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
  _store: new Map(),
  match(req: URL) {
    return Promise.resolve(this._store.get(req.href) || null);
  },
  put(req: URL, res: Response) {
    this._store.set(req.href, res);
    return Promise.resolve();
  },
};

const KV = {
  _store: new Map(),
  async getWithMetadata(
    key: string,
    _options: { type: "stream"; cacheTtl?: number },
  ): Promise<{ value: ReadableStream | null; metadata: any }> {
    const ret = this._store.get(key);
    if (ret) {
      return { value: new Response(ret.value).body!, metadata: ret.httpMetadata };
    }
    return { value: null, metadata: null };
  },
  async put(
    key: string,
    value: string | ArrayBufferLike | ArrayBuffer | ReadableStream,
    options?: { expirationTtl?: number; metadata?: any },
  ): Promise<void> {
    this._store.set(key, { value: await new Response(value).arrayBuffer(), ...options });
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
  (req) => worker.fetch(req, { ...env, KV, R2, LEGACY_WORKER }, { waitUntil: () => {} }),
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

    if (pathname === "/@private/hello/1.0.0.tgz") {
      try {
        const buf = Deno.readFileSync(join(dirname(new URL(import.meta.url).pathname), "hello-1.0.0.tgz"));
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

    if (pathname === "/@private/hello") {
      return Response.json({
        "name": "@private/hello",
        "description": "Hello world!",
        "dist-tags": {
          "latest": "1.0.0",
        },
        "versions": {
          "1.0.0": {
            "name": "@private/hello",
            "description": "Hello world!",
            "version": "1.0.0",
            "type": "module",
            "module": "dist/index.js",
            "types": "dist/index.d.ts",
            "files": [
              "dist/",
            ],
            "dist": {
              "tarball": "http://localhost:8082/@private/hello/1.0.0.tgz",
              // shasum -a 1 hello-1.0.0.tgz
              "shasum": "E308F75E8F8D4E67853C8BC11E66E217805FC7D7",
              // openssl dgst -binary -sha512 hello-1.0.0.tgz | openssl base64
              "integrity": "sha512-lgXANkhDdsvlhWaqrMN3L+d5S0X621h8NFrDA/V4eITPRUhH6YW3OWYG6NSa+n+peubBh7UHAXhtcsxdXUiYMA==",
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

    const res4 = await fetch(`${workerOrigin}/react~dom@18`);
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

  await t.step("embed polyfills/types", async () => {
    const res2 = await fetch(`${workerOrigin}/hot.d.ts`);
    assertEquals(res2.status, 200);
    assertEquals(res2.headers.get("Content-Type"), "application/typescript; charset=utf-8");
    assertEquals(res2.headers.get("Etag"), `W/"${version}"`);
    assertEquals(res2.headers.get("Cache-Control"), "public, max-age=86400");
    assertStringIncludes(await res2.text(), "export interface Hot");

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

    const res7 = await fetch(`${workerOrigin}/npm_node-fetch.js`);
    assertEquals(res7.status, 200);
    assertEquals(res7.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertEquals(res7.headers.get("Etag"), `W/"${version}"`);
    assertEquals(res7.headers.get("Cache-Control"), "public, max-age=86400");
    assertStringIncludes(res7.headers.get("Vary")!, "User-Agent");
    assertStringIncludes(await res7.text(), "fetch");

    const fs = await import(`${workerOrigin}/node/fs.js`);
    fs.writeFileSync("foo.txt", "bar", "utf8");
    assertEquals(fs.readFileSync("foo.txt", "utf8"), "bar");
  });

  await t.step("npm modules", async () => {
    const res = await fetch(`${workerOrigin}/react`, { redirect: "manual" });
    res.body?.cancel();
    assertEquals(res.status, 302);
    assert(res.headers.get("Location")!.startsWith(`${workerOrigin}/react@`));
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=600");

    const res2 = await fetch(res.headers.get("Location")!);
    const modUrl = new URL(res2.headers.get("x-esm-path")!, workerOrigin);
    res2.body?.cancel();
    assertEquals(res2.status, 200);
    assertEquals(res2.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertEquals(res2.headers.get("Cache-Control"), "public, max-age=31536000, immutable");
    assert(modUrl.pathname.endsWith("/denonext/react.mjs"));

    const res3 = await fetch(modUrl);
    assertEquals(res3.status, 200);
    assertEquals(res3.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertEquals(res3.headers.get("Cache-Control"), "public, max-age=31536000, immutable");
    assertStringIncludes(await res3.text(), "createElement");

    const dtsUrl = res2.headers.get("X-Typescript-Types")!;
    assert(dtsUrl.startsWith(workerOrigin));
    assert(dtsUrl.endsWith(".d.ts"));

    const res4 = await fetch(dtsUrl);
    res4.body?.cancel();
    assertEquals(res4.status, 200);
    assertEquals(res4.headers.get("Content-Type"), "application/typescript; charset=utf-8");

    const res5 = await fetch(`${workerOrigin}/react@^18.2.0`);
    assertEquals(res5.status, 200);
    assertEquals(res5.headers.get("Cache-Control"), "public, max-age=600");
    assertStringIncludes(await res5.text(), `"/react@18.`);

    const res6 = await fetch(`${workerOrigin}/react@17.0.2`);
    res6.body?.cancel();
    assertEquals(res6.status, 200);
    const modUrl2 = new URL(res6.headers.get("x-esm-path")!, workerOrigin);
    const res7 = await fetch(modUrl2);
    assertEquals(res7.status, 200);
    assertStringIncludes(
      await res7.text(),
      `"data:text/javascript;base64,ZXhwb3J0IGRlZmF1bHQgT2JqZWN0LmFzc2lnbg=="`,
    );

    const res8 = await fetch(`${workerOrigin}/react-dom@18.2.0?external=react`);
    res8.body?.cancel();
    assertEquals(res8.status, 200);
    const modUrl3 = new URL(res8.headers.get("x-esm-path")!, workerOrigin);
    const res9 = await fetch(modUrl3);
    assertEquals(res9.status, 200);
    assertStringIncludes(await res9.text(), `from "react"`);

    const res10 = await fetch(`${workerOrigin}/typescript@5.4.2/es2022/typescript.mjs`);
    assertEquals(res10.status, 200);
    assertStringIncludes(await res10.text(), `"/node/process.js"`);

    const res11 = await fetch(`${workerOrigin}/typescript@5.4.2/es2022/typescript.mjs.map`);
    assertEquals(res11.status, 200);
    assertEquals(res11.headers.get("Content-Type"), "application/json; charset=utf-8");
  });

  await t.step("npm modules (submodule)", async () => {
    const res = await fetch(`${workerOrigin}/react-dom@18/server`, {
      redirect: "manual",
    });
    res.body?.cancel();
    assertEquals(res.status, 302);
    assert(res.headers.get("Location")!.startsWith(`${workerOrigin}/react-dom@`));
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=600");

    const res2 = await fetch(res.headers.get("Location")!);
    const modUrl = new URL(res2.headers.get("x-esm-path")!, workerOrigin);
    res2.body?.cancel();
    assertEquals(res2.status, 200);
    assertEquals(res2.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertEquals(res2.headers.get("Cache-Control"), "public, max-age=31536000, immutable");
    assert(/\.d\.ts$/.test(res2.headers.get("X-Typescript-Types")!));
    assert(modUrl.pathname.endsWith("/denonext/server.js"));

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
    const res = await fetch(`${workerOrigin}/gh/microsoft/tslib`, { redirect: "manual" });
    res.body?.cancel();
    assertEquals(res.status, 302);
    const rUrl = res.headers.get("Location")!;
    assert(rUrl.startsWith(`${workerOrigin}/gh/microsoft/tslib@`));
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=600");
    const res2 = await fetch(rUrl);
    const modUrl = new URL(res2.headers.get("x-esm-path")!, workerOrigin);
    res2.body?.cancel();
    assertEquals(res2.status, 200);
    assertEquals(res2.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertEquals(res2.headers.get("Cache-Control"), "public, max-age=31536000, immutable");
    assert(/gh\/.+\.d\.ts$/.test(res2.headers.get("X-Typescript-Types")!));
    assert(modUrl.pathname.endsWith("/denonext/tslib.mjs"));

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
    const rUrl = res.headers.get("Location")!;
    assert(rUrl.startsWith(`${workerOrigin}/gh/microsoft/fluentui-emoji@`));
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=600");

    const res2 = await fetch(rUrl);
    const svg = await res2.text();
    assertEquals(res2.status, 200);
    assertEquals(res2.headers.get("Content-Type"), "image/svg+xml");
    assertEquals(res2.headers.get("Cache-Control"), "public, max-age=31536000, immutable");
    assertStringIncludes(svg, "<svg");
  });

  await t.step("jsr", async () => {
    const res = await fetch(
      `${workerOrigin}/jsr/@std/encoding/base64`,
      { redirect: "manual" },
    );
    res.body?.cancel();
    assertEquals(res.status, 302);
    const rUrl = res.headers.get("Location")!;
    assert(rUrl.startsWith(`${workerOrigin}/jsr/@std/encoding@`));
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=600");

    const res2 = await fetch(rUrl);
    const modUrl = new URL(res2.headers.get("x-esm-path")!, workerOrigin);
    res2.body?.cancel();
    assertEquals(res2.status, 200);
    assertEquals(res2.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertEquals(res2.headers.get("Cache-Control"), "public, max-age=31536000, immutable");
    assert(/@jsr\/std__encoding@.+\.d\.ts$/.test(res2.headers.get("X-Typescript-Types")!));
    assert(modUrl.pathname.endsWith("/denonext/base64.js"));

    const { encodeBase64, decodeBase64 } = await import(rUrl);
    assertEquals(encodeBase64("hello"), "aGVsbG8=");
    assertEquals(new TextDecoder().decode(decodeBase64("aGVsbG8=")), "hello");
  });

  await t.step("builtin scripts", async () => {
    const res = await fetch(`${workerOrigin}/hot`);
    res.body?.cancel();
    assertEquals(new URL(res.url).pathname, "/hot");
    assertEquals(res.headers.get("Etag"), `W/"${version}"`);
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=86400");
    assertEquals(res.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertStringIncludes(res.headers.get("Vary") ?? "", "User-Agent");
    const dtsUrl = res.headers.get("X-Typescript-Types")!;
    assert(dtsUrl.startsWith(workerOrigin));
    assert(dtsUrl.endsWith(".d.ts"));

    const res2 = await fetch(`${workerOrigin}/hot?target=es2022`);
    assertEquals(res2.headers.get("Etag"), `W/"${version}"`);
    assertEquals(res2.headers.get("Cache-Control"), "public, max-age=86400");
    assertEquals(res2.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertStringIncludes(await res2.text(), "esm.sh/hot");

    const res3 = await fetch(`${workerOrigin}/run`);
    assertEquals(res3.headers.get("Etag"), `W/"${version}"`);
    assertEquals(res3.headers.get("Cache-Control"), "public, max-age=86400");
    assertEquals(res3.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertStringIncludes(res3.headers.get("Vary") ?? "", "User-Agent");
    assertStringIncludes(await res3.text(), "esm.sh/run");
  });

  await t.step("transform api", async () => {
    const options = {
      code: `
        import { renderToString } from "preact-render-to-string";
        export default () => renderToString(<h1>Hello world!</h1>);
      `,
      filename: "source.jsx",
      target: "es2022",
      importMap: JSON.stringify({
        imports: {
          "@jsxImportSource": "https://preact@10.13.2",
          "preact-render-to-string": "https://esm.sh/preact-render-to-string6.0.2",
        },
      }),
      sourceMap: true,
    };
    const hash = await computeHash("jsx" + options.code + options.importMap + options.target + options.sourceMap);
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
    assertEquals(res.headers.get("Content-Type"), "application/json; charset=utf-8");
    const ret = await res.json();
    assert(Array.isArray(ret));
    assert(ret.length > 0);
  });

  await t.step("check esma target from user agent", async () => {
    const getTarget = async (ua: string) => {
      const res = await fetch(`${workerOrigin}/esma-target`, {
        headers: { "User-Agent": ua },
      });
      return await res.text();
    };
    assertEquals(await getTarget("Deno/1.33.1"), "deno");
    assertEquals(await getTarget("Deno/1.33.2"), "denonext");
  });

  await t.step("cache for different UAs", async () => {
    const fetchModule = async (pathname: string, ua: string) => {
      const res = await fetch(`${workerOrigin}` + pathname, {
        headers: { "User-Agent": ua },
      });
      assertStringIncludes(res.headers.get("Vary") ?? "", "User-Agent");
      return await res.text();
    };

    assertStringIncludes(await fetchModule("/react@18.2.0", "Deno/1.33.1"), "/deno/");
    assertStringIncludes(await fetchModule("/react@18.2.0", "Deno/1.33.2"), "/denonext/");
    assertStringIncludes(
      await fetchModule(
        "/react@18.2.0",
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Safari/537.36",
      ),
      "/es2022/",
    );
    assertStringIncludes(
      await fetchModule(
        "/react@18.2.0",
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.3 Safari/605.1.15",
      ),
      "/es2021/",
    );
    assertStringIncludes(await fetchModule("/react@18.2.0", "esm/es2022"), "/es2022/");
  });

  await t.step("fix urls", async () => {
    const res = await fetch(
      `${workerOrigin}/lightningcss-wasm@1.19.0/es2022/lightningcss_node.wasm`,
      { redirect: "manual" },
    );
    res.body?.cancel();
    assertEquals(res.status, 301);
    assertEquals(
      res.headers.get("location"),
      `${workerOrigin}/lightningcss-wasm@1.19.0/lightningcss_node.wasm`,
    );
  });

  await t.step("private registry", async () => {
    const res0 = await fetch(`http://localhost:8082/@private/hello`);
    res0.body?.cancel();
    assertEquals(res0.status, 401);

    const res1 = await fetch(`http://localhost:8082/@private/hello`, {
      headers: { authorization: "Bearer " + testRegisterToken },
    });
    assertEquals(res1.status, 200);
    const pkg = await res1.json();
    assertEquals(pkg.name, "@private/hello");

    const res2 = await fetch(`http://localhost:8082/@private/hello/1.0.0.tgz`);
    res2.body?.cancel();
    assertEquals(res2.status, 401);

    const res3 = await fetch(`http://localhost:8082/@private/hello/1.0.0.tgz`, {
      headers: { authorization: "Bearer " + testRegisterToken },
    });
    res3.body?.cancel();
    assertEquals(res3.status, 200);

    const { messsage } = await import(`${workerOrigin}/@private/hello`);
    assertEquals(messsage, "Hello world!");
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
  console.log(
    "KV",
    [...KV._store.keys()].map((k) => {
      const v = KV._store.get(k)!;
      if (v.expirationTtl) {
        return k + " (ttl/" + v.expirationTtl + ")";
      }
      return k + " (immutable)";
    }),
  );
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

import { serve } from "https://deno.land/std@0.180.0/http/server.ts";
import { join } from "https://deno.land/std@0.180.0/path/mod.ts";
import {
  assert,
  assertEquals,
  assertStringIncludes,
} from "https://deno.land/std@0.180.0/testing/asserts.ts";
import Worker from "../../packages/esm-worker/dist/index.js";

async function run(name: string, ...args: string[]) {
  const cwd = join(
    new URL(import.meta.url).pathname,
    "../../../packages/esm-worker",
  );
  const command = new Deno.Command(name, {
    args,
    stdin: "inherit",
    stdout: "inherit",
    cwd,
  });
  const status = await command.spawn().status;
  if (!status.success) {
    Deno.exit(status.code);
  }
}

await run("npm", "i");
await run("node", "build.mjs");

const workerOrigin = "http://localhost:8787";
const worker = Worker(
  async (_req: Request, ctx: { url: URL }) => {
    if (ctx.url.pathname === "/") {
      return new Response("<h1>Welcome to use esm.sh!</h1>", {
        headers: { "content-type": "text/html" },
      });
    }
  },
);
const env = {
  WORKER_ENV: "development",
  KV: {
    getWithMetadata: () => ({ value: null, metadata: null }),
    put: () => {},
  },
  R2: {
    get: () => null,
    put: () => {},
  },
  ESM_SERVER_ORIGIN: "http://localhost:8080",
};
const ac = new AbortController();

serve((req) => worker.fetch(req, env, { waitUntil: () => {} }), {
  port: 8787,
  signal: ac.signal,
});

await new Promise((resolve) => setTimeout(resolve, 500));

Deno.test("CF Worker", {
  sanitizeOps: false,
  sanitizeResources: false,
}, async (t) => {
  await t.step("custom homepage", async () => {
    const res = await fetch(workerOrigin, {
      headers: { "User-Agent": "Chrome/90.0.4430.212" },
    });
    assertEquals(res.status, 200);
    assertEquals(
      res.headers.get("Content-Type"),
      "text/html",
    );
    const text = await res.text();
    assertStringIncludes(text, "<h1>Welcome to use esm.sh!</h1>");
  });

  await t.step("deno CLI", async () => {
    const res = await fetch(workerOrigin);
    res.body?.cancel();
    assertEquals(res.status, 200);
    assertEquals(
      res.headers.get("Content-Type"),
      "application/typescript; charset=utf-8",
    );
  });

  await t.step("embed polyfills/types", async () => {
    const res = await fetch(`${workerOrigin}/v115/node.ns.d.ts`);
    res.body?.cancel();
    assertEquals(res.status, 200);
    assertEquals(
      res.headers.get("Content-Type"),
      "application/typescript; charset=utf-8",
    );

    const res2 = await fetch(`${workerOrigin}/v115/node_process.js`);
    res2.body?.cancel();
    assertEquals(res2.status, 200);
    assertStringIncludes(
      res2.headers.get("Content-Type")!,
      "/javascript; charset=utf-8",
    );
  });

  await t.step("status.json", async () => {
    const ret: any = await fetch(`${workerOrigin}/status.json`).then((res) =>
      res.json()
    );
    assertEquals(typeof ret.version, "number");
  });

  await t.step("npm modules", async () => {
    const res = await fetch(`${workerOrigin}/react-dom`, {
      redirect: "manual",
    });
    res.body?.cancel();
    assertEquals(res.status, 302);
    assertStringIncludes(res.headers.get("Location")!, "react-dom@");
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=600");
    const url = new URL(
      new URL(res.headers.get("Location")!).pathname,
      workerOrigin,
    );
    const res2 = await fetch(url);
    const code = await res2.text();
    const dts = res2.headers.get("X-Typescript-Types")!;
    assertEquals(res2.status, 200);
    assertEquals(
      res2.headers.get("Content-Type"),
      "application/javascript; charset=utf-8",
    );
    assertEquals(res2.headers.get("Cache-Control"), "public, max-age=86400");
    const modUrl = new URL(
      new URL(code.match(/from "(.+)"/)?.[1]!).pathname,
      workerOrigin,
    );
    assert(modUrl.pathname.endsWith("/deno/react-dom.mjs"));
    const res3 = await fetch(modUrl);
    assertEquals(res3.status, 200);
    assertEquals(
      res3.headers.get("Content-Type"),
      "application/javascript; charset=utf-8",
    );
    assertEquals(
      res3.headers.get("Cache-Control"),
      "public, max-age=31536000, immutable",
    );
    const modCode = await res3.text();
    assertStringIncludes(modCode, "/stable/react@");
    assertStringIncludes(modCode, "createElement");
    assert(/v\d+\/.+\.d\.ts$/.test(dts));
    const dtsUrl = new URL(new URL(dts).pathname, workerOrigin);
    const res4 = await fetch(dtsUrl);
    res4.body?.cancel();
    assertEquals(res4.status, 200);
    assertEquals(
      res4.headers.get("Content-Type"),
      "application/typescript; charset=utf-8",
    );
  });

  await t.step("npm modules (submodule)", async () => {
    const res = await fetch(`${workerOrigin}/react-dom@18/server`, {
      redirect: "manual",
    });
    res.body?.cancel();
    assertEquals(res.status, 302);
    assertStringIncludes(res.headers.get("Location")!, "react-dom@18");
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=600");
    const url = new URL(
      new URL(res.headers.get("Location")!).pathname,
      workerOrigin,
    );
    const res2 = await fetch(url);
    const code = await res2.text();
    assertEquals(res2.status, 200);
    assertEquals(
      res2.headers.get("Content-Type"),
      "application/javascript; charset=utf-8",
    );
    assertEquals(res2.headers.get("Cache-Control"), "public, max-age=86400");
    assert(/v\d+\/.+\.d\.ts$/.test(res2.headers.get("X-Typescript-Types")!));
    const modUrl = new URL(
      new URL(code.match(/from "(.+)"/)?.[1]!).pathname,
      workerOrigin,
    );
    assert(modUrl.pathname.endsWith("/deno/server.js"));
    const res3 = await fetch(modUrl);
    assertEquals(res3.status, 200);
    assertEquals(
      res3.headers.get("Content-Type"),
      "application/javascript; charset=utf-8",
    );
    assertEquals(
      res3.headers.get("Cache-Control"),
      "public, max-age=31536000, immutable",
    );
    assertStringIncludes(await res3.text(), "renderToString");
  });

  await t.step("npm modules (stable)", async () => {
    const res = await fetch(`${workerOrigin}/react`, { redirect: "manual" });
    res.body?.cancel();
    assertEquals(res.status, 302);
    assertStringIncludes(res.headers.get("Location")!, "react@");
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=600");
    const url = new URL(
      new URL(res.headers.get("Location")!).pathname,
      workerOrigin,
    );
    const res2 = await fetch(url);
    const code = await res2.text();
    const dts = res2.headers.get("X-Typescript-Types")!;
    assertEquals(res2.status, 200);
    assertEquals(
      res2.headers.get("Content-Type"),
      "application/javascript; charset=utf-8",
    );
    assertEquals(
      res2.headers.get("Cache-Control"),
      "public, max-age=31536000, immutable",
    );
    const modUrl = new URL(
      new URL(code.match(/from "(.+)"/)?.[1]!).pathname,
      workerOrigin,
    );
    assert(modUrl.pathname.startsWith("/stable/react@"));
    assert(modUrl.pathname.endsWith("/deno/react.mjs"));
    const res3 = await fetch(modUrl);
    assertEquals(res3.status, 200);
    assertEquals(
      res3.headers.get("Content-Type"),
      "application/javascript; charset=utf-8",
    );
    assertEquals(
      res3.headers.get("Cache-Control"),
      "public, max-age=31536000, immutable",
    );
    assertStringIncludes(await res3.text(), "createElement");
    assert(/v\d+\/.+\.d\.ts$/.test(dts));
    const dtsUrl = new URL(new URL(dts).pathname, workerOrigin);
    const res4 = await fetch(dtsUrl);
    res4.body?.cancel();
    assertEquals(res4.status, 200);
    assertEquals(
      res4.headers.get("Content-Type"),
      "application/typescript; charset=utf-8",
    );
  });

  await t.step("npm modules (pined)", async () => {
    const url = `${workerOrigin}/v115/react@18.2.0?target=es2020`;
    const res2 = await fetch(url);
    const code = await res2.text();
    const modUrl = new URL(
      new URL(code.match(/from "(.+)"/)?.[1]!).pathname,
      workerOrigin,
    );
    assertEquals(res2.status, 200);
    assertEquals(
      res2.headers.get("Content-Type"),
      "application/javascript; charset=utf-8",
    );
    assertEquals(
      res2.headers.get("Cache-Control"),
      "public, max-age=31536000, immutable",
    );
    assert(/v\d+\/.+\.d\.ts$/.test(res2.headers.get("X-Typescript-Types")!));
    assertEquals(modUrl.pathname, "/stable/react@18.2.0/es2020/react.mjs");
    const res3 = await fetch(modUrl);
    assertEquals(res3.status, 200);
    assertEquals(
      res3.headers.get("Content-Type"),
      "application/javascript; charset=utf-8",
    );
    assertEquals(
      res3.headers.get("Cache-Control"),
      "public, max-age=31536000, immutable",
    );
    assertStringIncludes(await res3.text(), "createElement");
  });

  await t.step("npm assets", async () => {
    const res = await fetch(`${workerOrigin}/react/package.json`, {
      redirect: "manual",
    });
    res.body?.cancel();
    assertEquals(res.status, 302);
    assertStringIncludes(res.headers.get("Location")!, "/react@");
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=600");
    const url = new URL(
      new URL(res.headers.get("Location")!).pathname,
      workerOrigin,
    );
    const res2 = await fetch(url);
    assertEquals(res2.status, 200);
    assertEquals(
      res2.headers.get("Content-Type"),
      "application/json",
    );
    assertEquals(
      res2.headers.get("Cache-Control"),
      "public, max-age=31536000, immutable",
    );
    const pkgJson: any = await res2.json();
    assertEquals(pkgJson.name, "react");
  });

  await t.step("gh assets", async () => {
    const res = await fetch(
      `${workerOrigin}/gh/microsoft/fluentui-emoji/assets/Alien/Flat/alien_flat.svg`,
      { redirect: "manual" },
    );
    res.body?.cancel();
    assertEquals(res.status, 302);
    assertStringIncludes(res.headers.get("Location")!, "fluentui-emoji@");
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=600");
    const url = new URL(
      new URL(res.headers.get("Location")!).pathname,
      workerOrigin,
    );
    const res2 = await fetch(url);
    const svg = await res2.text();
    assertEquals(res2.status, 200);
    assertEquals(res2.headers.get("Content-Type"), "image/svg+xml");
    assertEquals(
      res2.headers.get("Cache-Control"),
      "public, max-age=31536000, immutable",
    );
    assertStringIncludes(svg, "<svg");
  });

  await t.step("gh modules", async () => {
    const res = await fetch(`${workerOrigin}/gh/superfluid-finance/metadata`, {
      redirect: "manual",
    });
    res.body?.cancel();
    assertEquals(res.status, 302);
    assertStringIncludes(res.headers.get("Location")!, "metadata@");
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=600");
    const url = new URL(
      new URL(res.headers.get("Location")!).pathname,
      workerOrigin,
    );
    const res2 = await fetch(url);
    const code = await res2.text();
    const modUrl = new URL(
      new URL(code.match(/from "(.+)"/)?.[1]!).pathname,
      workerOrigin,
    );
    assertEquals(res2.status, 200);
    assertEquals(
      res2.headers.get("Content-Type"),
      "application/javascript; charset=utf-8",
    );
    assertEquals(res2.headers.get("Cache-Control"), "public, max-age=86400");
    assert(
      /v\d+\/gh\/.+\.d\.ts$/.test(res2.headers.get("X-Typescript-Types")!),
    );
    assert(modUrl.pathname.endsWith("/deno/metadata.mjs"));
    const res3 = await fetch(modUrl);
    assertEquals(res3.status, 200);
    assertEquals(
      res3.headers.get("Content-Type"),
      "application/javascript; charset=utf-8",
    );
    assertEquals(
      res3.headers.get("Cache-Control"),
      "public, max-age=31536000, immutable",
    );
    assertStringIncludes(await res3.text(), "export{");
  });

  await t.step("gh assets", async () => {
    const res = await fetch(
      `${workerOrigin}/gh/microsoft/fluentui-emoji/assets/Alien/Flat/alien_flat.svg`,
      { redirect: "manual" },
    );
    res.body?.cancel();
    assertEquals(res.status, 302);
    assertStringIncludes(res.headers.get("Location")!, "fluentui-emoji@");
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=600");
    const url = new URL(
      new URL(res.headers.get("Location")!).pathname,
      workerOrigin,
    );
    const res2 = await fetch(url);
    const svg = await res2.text();
    assertEquals(res2.status, 200);
    assertEquals(res2.headers.get("Content-Type"), "image/svg+xml");
    assertEquals(
      res2.headers.get("Cache-Control"),
      "public, max-age=31536000, immutable",
    );
    assertStringIncludes(svg, "<svg");
  });

  await t.step("build", async () => {
    const ret: any = await fetch(`${workerOrigin}/build`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        code: `/* @jsx h */
          import { h } from "preact@10.13.2";
          import { renderToString } from "preact-render-to-string@6.0.2";
          export default () => renderToString(<h1>Hello world!</h1>);
        `,
      }),
    }).then((r) => r.json());
    if (ret.error) {
      throw new Error(`<${ret.error.status}> ${ret.error.message}`);
    }
    const { default: render } = await import(
      new URL(`/v119${new URL(ret.url).pathname}/deno/mod.mjs`, workerOrigin)
        .href
    );
    assertEquals(render(), "<h1>Hello world!</h1>");
  });

  ac.abort();
  await new Promise((resolve) => setTimeout(resolve, 500));
});

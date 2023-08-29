import { serve } from "https://deno.land/std@0.180.0/http/server.ts";
import { join } from "https://deno.land/std@0.180.0/path/mod.ts";
import {
  assert,
  assertEquals,
  assertStringIncludes,
} from "https://deno.land/std@0.180.0/testing/asserts.ts";

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
    throw new Error(`Failed to run ${name} ${args.join(" ")}`);
  }
}

// build and import esm worker
await run("pnpm", "i");
await run("node", "build.mjs");
const { withESMWorker } = await import(
  `../../packages/esm-worker/dist/index.js`
);

const workerOrigin = "http://localhost:8787";
const worker = withESMWorker(
  (_req: Request, _env: {}, ctx: { url: URL }) => {
    if (ctx.url.pathname === "/") {
      return new Response("<h1>Welcome to esm.sh!</h1>", {
        headers: { "content-type": "text/html" },
      });
    }
  },
);
const env = {
  ESM_ORIGIN: "http://localhost:8080",
};
const ac = new AbortController();

// start worker
serve((req) => worker.fetch(req, env, { waitUntil: () => {} }), {
  port: 8787,
  signal: ac.signal,
});

// wait for a while
await new Promise((resolve) => setTimeout(resolve, 500));

Deno.test("esm-worker", {
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
    assertStringIncludes(text, "<h1>Welcome to esm.sh!</h1>");
  });

  let VERSION: number;
  await t.step("status.json", async () => {
    const ret: any = await fetch(`${workerOrigin}/status.json`).then((res) =>
      res.json()
    );
    assertEquals(typeof ret.version, "number");
    VERSION = ret.version;
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
    const res = await fetch(`${workerOrigin}/v${VERSION}/node.ns.d.ts`);
    res.body?.cancel();
    assertEquals(res.status, 200);
    assertEquals(
      res.headers.get("Content-Type"),
      "application/typescript; charset=utf-8",
    );

    const res2 = await fetch(`${workerOrigin}/v${VERSION}/node_process.js`);
    res2.body?.cancel();
    assertEquals(res2.status, 200);
    assertStringIncludes(
      res2.headers.get("Content-Type")!,
      "/javascript; charset=utf-8",
    );
  });

  await t.step("npm modules", async () => {
    const res = await fetch(`${workerOrigin}/react-dom`, {
      redirect: "manual",
    });
    res.body?.cancel();
    assertEquals(res.status, 302);
    assert(
      res.headers.get("Location")!.startsWith(`${workerOrigin}/react-dom@`),
    );
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=600");
    const res2 = await fetch(res.headers.get("Location")!);
    const modUrl = new URL(res2.headers.get("X-Esm-Id")!, workerOrigin);
    res2.body?.cancel();
    assertEquals(res2.status, 200);
    assertEquals(
      res2.headers.get("Content-Type"),
      "application/javascript; charset=utf-8",
    );
    assertEquals(res2.headers.get("Cache-Control"), "public, max-age=86400");
    assert(modUrl.pathname.endsWith("/denonext/react-dom.mjs"));
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

    const dtsUrl = res2.headers.get("X-Typescript-Types")!;
    assert(dtsUrl.startsWith(workerOrigin));
    assert(dtsUrl.endsWith(".d.ts"));
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
    assert(
      res.headers.get("Location")!.startsWith(`${workerOrigin}/react-dom@`),
    );
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=600");
    const res2 = await fetch(res.headers.get("Location")!);
    const modUrl = new URL(res2.headers.get("X-Esm-Id")!, workerOrigin);
    res2.body?.cancel();
    assertEquals(res2.status, 200);
    assertEquals(
      res2.headers.get("Content-Type"),
      "application/javascript; charset=utf-8",
    );
    assertEquals(res2.headers.get("Cache-Control"), "public, max-age=86400");
    assert(/v\d+\/.+\.d\.ts$/.test(res2.headers.get("X-Typescript-Types")!));
    assert(modUrl.pathname.endsWith("/denonext/server.js"));
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
    assert(res.headers.get("Location")!.startsWith(`${workerOrigin}/react@`));
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=600");
    const res2 = await fetch(res.headers.get("Location")!);
    const modUrl = new URL(res2.headers.get("X-Esm-Id")!, workerOrigin);
    res2.body?.cancel();
    assertEquals(res2.status, 200);
    assertEquals(
      res2.headers.get("Content-Type"),
      "application/javascript; charset=utf-8",
    );
    assertEquals(
      res2.headers.get("Cache-Control"),
      "public, max-age=31536000, immutable",
    );
    assertEquals(modUrl.origin, workerOrigin);
    assert(modUrl.pathname.startsWith("/stable/react@"));
    assert(modUrl.pathname.endsWith("/denonext/react.mjs"));
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

    const dtsUrl = res2.headers.get("X-Typescript-Types")!;
    assert(dtsUrl.startsWith(workerOrigin));
    assert(dtsUrl.endsWith(".d.ts"));
    const res4 = await fetch(dtsUrl);
    res4.body?.cancel();
    assertEquals(res4.status, 200);
    assertEquals(
      res4.headers.get("Content-Type"),
      "application/typescript; charset=utf-8",
    );
  });

  await t.step("npm modules (pined)", async () => {
    const url = `${workerOrigin}/v${VERSION}/react@18.2.0?target=es2020`;
    const res2 = await fetch(url);
    const modUrl = new URL(res2.headers.get("X-Esm-Id")!, workerOrigin);
    res2.body?.cancel();
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
    assertEquals(modUrl.origin, workerOrigin);
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
    assert(res.headers.get("Location")!.startsWith(`${workerOrigin}/react@`));
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=600");
    const res2 = await fetch(res.headers.get("Location")!);
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

  await t.step("gh modules", async () => {
    const res = await fetch(`${workerOrigin}/gh/microsoft/tslib`, {
      redirect: "manual",
    });
    res.body?.cancel();
    assertEquals(res.status, 302);
    const rUrl = res.headers.get("Location")!;
    assert(rUrl.startsWith(`${workerOrigin}/gh/microsoft/tslib@`));
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=600");
    const res2 = await fetch(res.headers.get("Location")!);
    const modUrl = new URL(res2.headers.get("X-Esm-Id")!, workerOrigin);
    res2.body?.cancel();
    assertEquals(res2.status, 200);
    assertEquals(
      res2.headers.get("Content-Type"),
      "application/javascript; charset=utf-8",
    );
    assertEquals(res2.headers.get("Cache-Control"), "public, max-age=86400");
    assert(
      /v\d+\/gh\/.+\.d\.ts$/.test(res2.headers.get("X-Typescript-Types")!),
    );
    assert(modUrl.pathname.endsWith("/denonext/tslib.mjs"));
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
    const rUrl = res.headers.get("Location")!;
    assert(rUrl.startsWith(`${workerOrigin}/gh/microsoft/fluentui-emoji@`));
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=600");
    const res2 = await fetch(rUrl);
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
    const res = await fetch(`${workerOrigin}/build`);
    res.body?.cancel();
    assertEquals(new URL(res.url).pathname, `/v${VERSION}/build`);
    assertEquals(
      res.headers.get("Cache-Control"),
      "public, max-age=31536000, immutable",
    );
    assertEquals(
      res.headers.get("Content-Type"),
      "application/typescript; charset=utf-8",
    );

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
      new URL(
        `/v${VERSION}${new URL(ret.url).pathname}/denonext/mod.mjs`,
        workerOrigin,
      )
        .href
    );
    assertEquals(render(), "<h1>Hello world!</h1>");
  });

  await t.step("/esma-target", async () => {
    const getTarget = async (ua: string) => {
      const rest = await fetch(`${workerOrigin}/esma-target`, {
        headers: { "User-Agent": ua },
      });
      return await rest.text();
    };
    assertEquals(await getTarget("Deno/1.33.1"), "deno");
    assertEquals(await getTarget("Deno/1.33.2"), "denonext");
    assertEquals(
      await getTarget(
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Safari/537.36",
      ),
      "es2022",
    );
    assertEquals(
      await getTarget(
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.3 Safari/605.1.15",
      ),
      "es2021",
    );
  });

  ac.abort();
  await new Promise((resolve) => setTimeout(resolve, 500));
});

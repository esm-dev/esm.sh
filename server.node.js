//This Code can be run with Reejs@(Node/Bun)
//Currently Bun doesn't have (node:fs).fdatasync so wait until its implemented.

import { serve } from "https://esm.sh/@hono/node-server@1.0.2";
import { Hono } from "https://esm.sh/hono@3.2.7";
import crypto from "node:crypto";
import { join } from "https://deno.land/std@0.188.0/path/mod.ts";
import { ensureDir } from "https://deno.land/std@0.188.0/fs/ensure_dir.ts";
import { parse } from "https://deno.land/std@0.188.0/flags/mod.ts";

import { caching } from 'https://esm.sh/cache-manager@5.2.3';

const envKeys = [
  "ESM_ORIGIN",
  "ESM_TOKEN",
  "NPM_REGISTRY",
  "NPM_TOKEN",
];

let env = {};
envKeys.forEach((key) => {
  if (!env[key]) {
    const value = Deno.env.get(key);
    if (value) {
      Reflect.set(env, key, value);
    }
  }
});

//setup cache
const Cache = await caching('memory', {
  max: 100,
  ttl: 10 * 1000 /*milliseconds*/,
});

//ensure cache dir
await ensureDir(await getGlobalCacheDir());

async function getGlobalCacheDir() {
  // home dir/.cache/esm.sh
  const homeDir = Deno.env.get("HOME");
  return join(homeDir, ".cache", "esm.sh");
}

async function hashKey(key) {
  const buffer = await crypto.subtle.digest(
    "SHA-256",
    new TextEncoder().encode(key),
  );
  // return hex string
  return [...new Uint8Array(buffer)].map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}

let flags = parse(Deno.args);//returns: { _: [ 'x', 'index.js', 'h' ] }
//remove the first 2 elements from _
flags._.shift();
flags._.shift();
flags = Object.assign(flags, flags._.reduce((acc, key) => {
  acc[key] = true;
  return acc;
}, {}));

delete flags._;

if (flags.help || flags.h) {
  console.log(
    "%cWelcome to local esm.sh!",
    "font-weight:bold;color:#007bff;",
  );
  console.log(
    "%cThis is local version of esm.sh running on %cReejs 🔮%c.",
    "color:gray;", "color: #805ad5", "color:gray;"
  );
  console.log("");
  console.log("Usage:");
  console.log(`  reejs x https://esm.sh/server`);
  console.log("");
  console.log("Options:");
  console.log("  --port <port>    Port to listen on. Default is 8787.");
  console.log("  --help, -h       Print this help message.");
  console.log("");
  console.log("ENVIRONMENT VARIABLES:");
  console.log("  ESM_ORIGIN    The origin of esm.sh server.");
  console.log("  ESM_TOKEN     The token of esm.sh server.");
  console.log("  NPM_REGISTRY  The npm registry, Default is 'https://registry.npmjs.org/'.");
  console.log("  NPM_TOKEN     The npm token.");
  process.exit(0);
}

const port = flags.port || Deno.env.get("PORT") || 8787;
const App = new Hono();
App.get("/", (c) => {
  let url = new URL(c.req.url);
  return new Response(
    `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>ESM&gt;CDN</title>
  <style>
    * {
      padding: 0;
      margin: 0;
    }
    body {
      display: flex;
      align-items: center;
      justify-content: center;
      flex-direction: column;
      height: 100vh;
      overflow: hidden;
    }
    h1 {
      margin-bottom: 8px;
      font-size: 40px;
      line-height: 1;
      font-family: Inter,sans-serif;
      text-align: center;
    }
    p {
      font-size: 18px;
      line-height: 1;
      color: #888;
      text-align: center:
    }
    pre {
      margin-top: 24px;
      background-color: #f3f3f3;
      padding: 18px;
      border-radius: 8px;
      font-size: 14px;
      line-height: 1;
    }
    pre .keyword {
      color: #888;
    }
    pre a {
      color: #58a65c;
      text-decoration: underline;
    }
    pre a:hover {
      opacity: 0.8;
    }
  </style>
</head>
<body>
  <h1>Welcome to local esm.sh!</h1>
  <p>This is local version of esm.sh running on Reejs 🔮.</p>
  <pre><code><span class="keyword">import</span> react <span class="keyword">from</span> "<a href="${url.origin}/react" class="url">${url.origin}/react</a>"</code></pre>
</body>
</html>
`, {
    headers: {
      "content-type": "text/html; charset=utf-8",
    },
  });
});
App.on(["GET", "POST"], "*", async (c) => {
  let url = new URL(c.req.url);
  let req = c.req.raw;
  let cacheKey = await hashKey(req.url);
  let cache = await Cache.get(`${req.method}-${cacheKey}`);
  if (cache) {
    return new Response(cache.body, {
      headers: cache.headers,
    });
  }
  //send request to esm.sh
  let proxyRes = await fetch(`https://esm.sh${url.pathname}${url.search}`, {
    method: req.method,
    headers: {
      "User-Agent": req.headers.get("user-agent"),
      "Accept": req.headers.get("accept"),
      "Accept-Encoding": req.headers.get("accept-encoding"),
      "X-Real-Origin": url.origin,
    },
    body: req.body,
  });

  let resHeaders = proxyRes.headers;
  let body = await proxyRes.text();
  //body = body.replace(/https:\/\/esm.sh/g, url.origin);
  //save cache
  //check cache
  cacheKey = await hashKey(proxyRes.url);
  let ttl = resHeaders.get("cache-control")?.match(/max-age=(\d+)/)?.[1] || 60 * 60 * 24 * 30;
  let headers = {
    "content-type": resHeaders.get("content-type"),
    "content-length": resHeaders.get("content-length"),
    "cache-control": resHeaders.get("cache-control"),
    "location": resHeaders.get("location"),
    "server": resHeaders.get("server"),
    "Cf-Cache-Status": "HIT",
    "Access-Control-Allow-Origin": "*",
    "Access-Control-Allow-Methods": "*",
    "Report-To": resHeaders.get("Report-To"),
    "X-Content-Source": resHeaders.get("X-Content-Source"),
    "Access-Control-Expose-Headers": "X-TypeScript-Types",
    "X-TypeScript-Types": resHeaders.get("X-TypeScript-Types"),
  };
  await Cache.set(`${req.method}-${cacheKey}`, {
    body,
    headers
  }, ttl);
  //check if it is a redirect
  if (proxyRes.url !== url.href) {
    //save redirect cache
    cacheKey = await hashKey(url.href);
    await Cache.set(`${req.method}-${cacheKey}`, {
      body,
      headers
    }, ttl);
    return c.redirect(proxyRes.url.replace(/https:\/\/esm.sh/g, url.origin)
      , 302);
  }
  //get response body
  return new Response(body, {
    headers,
  });

});

console.log(
  "%cWelcome to local esm.sh!",
  "font-weight:bold;color:#007bff;",
);
console.log(
  "%cThis is local version of esm.sh running on %cReejs 🔮%c.",
  "color:gray;", "color: #805ad5", "color:gray;"
);
console.log(
  `Homepage: %chttp://localhost:${port}`,
  "color:gray;",
  "",
);
console.log(
  `Module: %chttp://localhost:${port}/react`,
  "color:gray;",
  "",
);

if (!globalThis?.Bun) {
  
  serve({
    fetch: App.fetch,
    port,
  });
}

export default App;

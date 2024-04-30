/// <reference types="@cloudflare/workers-types" />

import { compareVersions, satisfies, validate } from "compare-versions";
import { getBuildTargetFromUA, targets } from "esm-compat";
import type {
  Context,
  HttpMetadata,
  Middleware,
  PackageInfo,
  PackageRegistryInfo,
  WorkerStorage,
} from "../types/index.d.ts";
import { assetsExts, cssPackages, VERSION } from "./consts.ts";
import { getContentType } from "./media_type.ts";
import {
  asKV,
  checkPreflight,
  copyHeaders,
  corsHeaders,
  err,
  errPkgNotFound,
  hashText,
  hasTargetSegment,
  isDtsFile,
  normalizeSearchParams,
  redirect,
  splitBy,
  trimPrefix,
} from "./utils.ts";

const regexpNpmNaming = /^[a-zA-Z0-9][\w\-\.]*$/;
const regexpFullVersion = /^\d+\.\d+\.\d+/;
const regexpCaretVersion = /^\^\d+\.\d+\.\d+/;
const regexpCommitish = /^[a-f0-9]{10,}$/;
const regexpLegacyVersionPrefix = /^\/v\d+\//;
const regexpLegacyBuild = /^\/~[a-f0-9]{40}$/;

const version = `v${VERSION}`;
const defaultNpmRegistry = "https://registry.npmjs.org";
const defaultEsmServerOrigin = "https://esm.sh";
const immutableCache = "public, max-age=31536000, immutable";

const dummyCache: Cache = {
  match: () => Promise.resolve(null),
  put: () => Promise.resolve(),
} as any;
const dummyStorage: WorkerStorage = {
  get: () => Promise.resolve(null),
  put: () => Promise.resolve(),
};

async function fetchOrigin(
  req: Request,
  env: Env,
  ctx: Context,
  uri: string,
  resHeaders: Headers,
): Promise<Response> {
  const headers = new Headers();
  copyHeaders(
    headers,
    req.headers,
    "Content-Type",
    "Referer",
    "User-Agent",
    "X-Forwarded-For",
    "X-Real-Ip",
    "X-Real-Origin",
  );
  if (!headers.has("X-Real-Origin")) {
    headers.set("X-Real-Origin", ctx.url.origin);
  }
  if (env.ESM_TOKEN) {
    headers.set("Authorization", `Bearer ${env.ESM_TOKEN}`);
  }
  const res = await fetch(
    new URL(uri, env.ESM_ORIGIN ?? defaultEsmServerOrigin),
    {
      method: req.method === "HEAD" ? "GET" : req.method,
      body: req.body,
      headers,
      redirect: "manual",
    },
  );
  const buffer = await res.arrayBuffer();
  if (!res.ok) {
    // CF default error page(html)
    if (res.status === 500 && res.headers.get("Content-Type")?.startsWith("text/html")) {
      return new Response("Bad Gateway", { status: 502, headers: resHeaders });
    }
    // redirects
    if (res.status === 301 || res.status === 302) {
      return redirect(res.headers.get("Location")!, res.status);
    }
    // fix cache-control by status code
    if (res.headers.has("Cache-Control")) {
      resHeaders.set("Cache-Control", res.headers.get("Cache-Control")!);
    } else if (res.status === 400) {
      resHeaders.set("Cache-Control", immutableCache);
    }
    copyHeaders(resHeaders, res.headers, "Content-Type");
    return new Response(buffer, { status: res.status, headers: resHeaders });
  }
  copyHeaders(
    resHeaders,
    res.headers,
    "Cache-Control",
    "Content-Type",
    "Content-Length",
    "X-Esm-Id",
    "X-Typescript-Types",
  );
  const exposedHeaders: string[] = [];
  for (const key of ["X-Esm-Id", "X-Typescript-Types"]) {
    if (resHeaders.has(key)) {
      exposedHeaders.push(key);
    }
  }
  if (exposedHeaders.length > 0) {
    resHeaders.set("Access-Control-Expose-Headers", exposedHeaders.join(", "));
  }
  return new Response(buffer, { headers: resHeaders });
}

async function fetchOriginWithKVCache(
  req: Request,
  env: Env,
  ctx: Context,
  pathname: string,
  query?: string,
  gzip?: boolean,
): Promise<Response> {
  let uri = pathname;
  if (query) {
    uri += query;
  }
  let storeKey = uri.slice(1);
  const headers = corsHeaders();
  const R2 = Reflect.get(env, "R2") as R2Bucket | undefined ?? dummyStorage;
  const KV = Reflect.get(env, "KV") as KVNamespace | undefined ?? asKV(R2);
  const fromWorker = req.headers.has("X-Real-Origin");
  const isRaw = ctx.url.searchParams.has("raw");
  const isDts = isDtsFile(pathname);
  const isModule = !(isRaw || isDts || pathname.endsWith(".mjs.map") || pathname.endsWith(".js.map"));
  if (!isModule) {
    storeKey = pathname.slice(1);
    if (storeKey.startsWith("*") && (isRaw || isDts)) {
      storeKey = storeKey.slice(1);
    }
  }

  if (!fromWorker) {
    if (isModule) {
      const { value, metadata } = await KV.getWithMetadata<HttpMetadata>(
        storeKey,
        { type: "stream", cacheTtl: 86400 },
      );
      if (value && metadata) {
        let body = value as ReadableStream<Uint8Array>;
        if (gzip && typeof DecompressionStream !== "undefined") {
          body = body.pipeThrough(new DecompressionStream("gzip"));
        }
        headers.set("Content-Type", metadata.contentType);
        headers.set("Cache-Control", immutableCache);
        const exposedHeaders: string[] = [];
        if (metadata.esmId) {
          headers.set("X-Esm-Id", metadata.esmId);
          exposedHeaders.push("X-Esm-Id");
        }
        if (metadata.dts) {
          headers.set("X-TypeScript-Types", metadata.dts);
          exposedHeaders.push("X-TypeScript-Types");
        }
        if (exposedHeaders.length > 0) {
          headers.set(
            "Access-Control-Expose-Headers",
            exposedHeaders.join(", "),
          );
        }
        headers.set("X-Content-Source", "esm-worker");
        return new Response(body, { headers });
      }
    } else {
      const obj = await R2.get(storeKey);
      if (obj) {
        const contentType = obj.httpMetadata?.contentType || getContentType(pathname);
        headers.set("Content-Type", contentType);
        headers.set("Cache-Control", immutableCache);
        headers.set("X-Content-Source", "esm-worker");
        return new Response(obj.body, { headers });
      }
    }
  }

  const res = await fetchOrigin(req, env, ctx, uri, headers);
  if (!res.ok) {
    return res;
  }

  const buffer = await res.arrayBuffer();
  const contentType = res.headers.get("Content-Type") || getContentType(pathname);
  const cacheControl = res.headers.get("Cache-Control");
  const esmId = res.headers.get("X-Esm-Id") ?? undefined;
  const dts = res.headers.get("X-TypeScript-Types") ?? undefined;
  const exposedHeaders: string[] = [];

  headers.set("Content-Type", contentType);
  if (cacheControl) {
    headers.set("Cache-Control", cacheControl);
  }
  if (esmId) {
    headers.set("X-Esm-Id", esmId);
    exposedHeaders.push("X-Esm-Id");
  }
  if (dts) {
    headers.set("X-TypeScript-Types", dts);
    exposedHeaders.push("X-TypeScript-Types");
  }
  if (exposedHeaders.length > 0) {
    headers.set("Access-Control-Expose-Headers", exposedHeaders.join(", "));
  }
  headers.set("X-Content-Source", "origin-server");

  // save to KV/R2 if immutable
  if (!fromWorker && cacheControl?.includes("immutable")) {
    if (!isModule) {
      ctx.waitUntil(R2.put(storeKey, buffer.slice(0), {
        httpMetadata: { contentType },
      }));
    } else {
      let value: ArrayBuffer | ReadableStream = buffer.slice(0);
      if (gzip && typeof CompressionStream !== "undefined") {
        value = new Response(value).body!.pipeThrough<Uint8Array>(
          new CompressionStream("gzip"),
        );
      }
      ctx.waitUntil(KV.put(storeKey, value, {
        metadata: { contentType, dts, esmId },
      }));
    }
  }

  return new Response(buffer, { headers });
}

async function fetchOriginWithR2Cache(
  req: Request,
  ctx: Context,
  env: Env,
  pathname: string,
): Promise<Response> {
  const resHeaders = corsHeaders();
  const r2 = Reflect.get(env, "R2") as R2Bucket | undefined ?? dummyStorage;
  const ret = await r2.get(pathname.slice(1));
  if (ret) {
    resHeaders.set(
      "Content-Type",
      ret.httpMetadata?.contentType || getContentType(pathname),
    );
    resHeaders.set("Cache-Control", immutableCache);
    resHeaders.set("X-Content-Source", "esm-worker");
    return new Response(ret.body as ReadableStream<Uint8Array>, {
      headers: resHeaders,
    });
  }

  const res = await fetchOrigin(req, env, ctx, pathname, resHeaders);
  if (res.ok) {
    const contentType = res.headers.get("content-type") ||
      getContentType(pathname);
    const buffer = await res.arrayBuffer();
    ctx.waitUntil(r2.put(pathname.slice(1), buffer.slice(0), {
      httpMetadata: { contentType },
    }));
    resHeaders.set("Content-Type", contentType);
    resHeaders.set("Cache-Control", immutableCache);
    resHeaders.set("X-Content-Source", "origin-server");
    return new Response(buffer, { headers: resHeaders });
  }
  return res;
}

function withESMWorker(middleware?: Middleware, cache: Cache = (caches as any).default ?? dummyCache) {
  async function handler(req: Request, env: Env, cfCtx: ExecutionContext): Promise<Response> {
    const resp = checkPreflight(req);
    if (resp) {
      return resp;
    }

    const url = new URL(req.url);
    const ua = req.headers.get("User-Agent");
    const withCache: Context["withCache"] = async (fetcher, options) => {
      const { pathname, searchParams } = url;
      const isHeadMethod = req.method === "HEAD";
      const hasPinedTarget = targets.has(searchParams.get("target") ?? "");
      const cacheKey = new URL(url);
      const varyUA = options?.varyUA && !hasPinedTarget && !isDtsFile(pathname) && !searchParams.has("raw");
      if (varyUA) {
        const target = getBuildTargetFromUA(ua);
        cacheKey.searchParams.set("target", target);
        searchParams.set("target", target);
      }
      const realOrigin = req.headers.get("X-REAL-ORIGIN");
      if (realOrigin) {
        cacheKey.searchParams.set("X-REAL-ORIGIN", realOrigin);
      }
      let res = await cache.match(cacheKey);
      if (res) {
        if (isHeadMethod) {
          const { status, headers } = res;
          return new Response(null, { status, headers });
        }
        return res;
      }
      res = await fetcher();
      // since we add `?target` to the search params, the "User-Agent" part of `vary` header from the origin server is missed,
      // so we need to add it manually
      if (varyUA) {
        const headers = new Headers(res.headers);
        headers.append("Vary", "User-Agent");
        res = new Response(res.body, { status: res.status, headers });
      }
      if (res.ok && res.headers.get("Cache-Control")?.startsWith("public, max-age=")) {
        cfCtx.waitUntil(cache.put(cacheKey, res.clone()));
      }
      if (isHeadMethod) {
        const { status, headers } = res;
        return new Response(null, { status, headers });
      }
      return res;
    };
    const ctx: Context = {
      cache,
      url,
      data: {},
      waitUntil: (p: Promise<any>) => cfCtx.waitUntil(p),
      withCache,
    };

    let pathname = url.pathname;

    // ban malicious requests
    if (pathname.startsWith("/.") || pathname.endsWith(".php")) {
      return ctx.withCache(() =>
        new Response(null, {
          status: 404,
          headers: { "cache-control": immutableCache },
        })
      );
    }

    // strip trailing slash
    if (pathname !== "/" && pathname.endsWith("/")) {
      pathname = pathname.slice(0, -1);
    }

    if (req.method === "POST" && pathname === "/transform") {
      return await fetchOrigin(
        new Request(req.url, {
          method: "POST",
          headers: req.headers,
          body: req.body,
        }),
        env,
        ctx,
        "/transform",
        corsHeaders(),
      );
    }

    switch (pathname) {
      case "/error.js":
        return ctx.withCache(() => fetchOrigin(req, env, ctx, pathname + url.search, corsHeaders()));

      case "/status.json":
        return fetchOrigin(req, env, ctx, pathname, corsHeaders());

      case "/esma-target":
        return ctx.withCache(
          () => {
            const headers = corsHeaders();
            headers.set("cache-control", immutableCache);
            return new Response(getBuildTargetFromUA(ua), { headers });
          },
          { varyUA: true },
        );
    }

    if (
      pathname === "/run" ||
      pathname === "/hot" ||
      pathname === "/node.ns.d.ts" ||
      pathname === "/hot.d.ts" ||
      ((pathname.startsWith("/node/") || pathname.startsWith("/npm_")) && pathname.endsWith(".js"))
    ) {
      const etag = `W/"${version}"`;
      const varyUA = !pathname.endsWith(".ts") && !pathname.endsWith(".js");
      const isChunkjs = pathname.startsWith("/node/chunk-");
      if (!isChunkjs) {
        const ifNoneMatch = req.headers.get("If-None-Match");
        if (ifNoneMatch === etag) {
          const headers = corsHeaders();
          headers.set("Cache-Control", "public, max-age=86400");
          return new Response(null, { status: 304, headers: corsHeaders() });
        }
        url.searchParams.set("v", VERSION.toString());
      }
      const res = await ctx.withCache(() => {
        const target = url.searchParams.get("target");
        return fetchOriginWithKVCache(req, env, ctx, pathname, target ? `?target=${target}` : void 0);
      }, { varyUA });
      if (!res.ok) {
        return res;
      }
      const headers = new Headers(res.headers);
      if (isChunkjs) {
        headers.set("Cache-Control", "public, max-age=31536000, immutable");
      } else {
        headers.set("Cache-Control", "public, max-age=86400");
        headers.set("Etag", etag);
      }
      return new Response(res.body, { status: res.status, headers });
    }

    if (middleware) {
      const resp = await middleware(req, env, ctx);
      if (resp) {
        return resp;
      }
    }

    if (req.method !== "GET" && req.method !== "HEAD") {
      return err("Method Not Allowed", 405);
    }

    // return 404 for favicon.ico and robots.txt
    if (pathname === "/favicon.ico" || pathname === "/robots.txt") {
      return err("Not Found", 404);
    }

    // use the default landing page/embedded files
    if (pathname === "/" || pathname.startsWith("/embed/")) {
      return fetchOrigin(req, env, ctx, `${pathname}${url.search}`, corsHeaders());
    }

    // if it's a singleton build module which is created by https://esm.sh/run
    if (pathname.startsWith("/+") && pathname.endsWith(".mjs")) {
      return ctx.withCache(
        () => {
          const target = url.searchParams.get("target");
          return fetchOriginWithKVCache(req, env, ctx, pathname, target ? `?target=${target}` : void 0);
        },
        { varyUA: true },
      );
    }

    // use legacy worker if the bild version is specified in the path or query
    if (env.LEGACY_WORKER) {
      if (
        pathname == "/build" ||
        url.searchParams.has("pin") ||
        pathname.startsWith("/stable/") ||
        (pathname.startsWith("/v") && regexpLegacyVersionPrefix.test(pathname)) ||
        (pathname.startsWith("/~") && regexpLegacyBuild.test(pathname))
      ) {
        return env.LEGACY_WORKER.fetch(req.clone());
      }
    }

    // decode pathname
    pathname = decodeURIComponent(pathname);

    // fix `/jsx-runtime` suffix in query, normally it happens with import maps
    if (
      url.search.endsWith("/jsx-runtime") ||
      url.search.endsWith("/jsx-dev-runtime")
    ) {
      const [q, jsxRuntime] = splitBy(url.search, "/", true);
      pathname = pathname + "/" + jsxRuntime;
      url.pathname = pathname;
      url.search = q;
    }

    // strip loc
    if (/:\d+:\d+$/.test(pathname)) {
      pathname = splitBy(pathname, ":")[0];
    }

    const gh = pathname.startsWith("/gh/");
    if (gh) {
      pathname = "/@" + pathname.slice(4);
    } else if (pathname.startsWith("/jsr/@")) {
      const segs = pathname.split("/");
      pathname = "/@jsr/" + segs[2].slice(1) + "__" + segs[3];
      if (segs.length > 4) {
        pathname += "/" + segs.slice(4).join("/");
      }
    }

    // strip external all marker
    const hasExternalAllMarker = pathname.startsWith("/*");
    if (hasExternalAllMarker) {
      pathname = "/" + pathname.slice(2);
    }

    let packageScope = "";
    let packageName = "";
    let packageVersion = "";
    let subPath = "";
    let extraQuery = "";

    if (pathname.startsWith("/@")) {
      const [scope, name, ...rest] = decodeURIComponent(pathname).slice(2).split("/");
      packageScope = "@" + scope;
      [packageName, packageVersion] = splitBy(name, "@");
      if (rest.length > 0) {
        subPath = "/" + rest.join("/");
      }
    } else {
      const [name, ...rest] = decodeURIComponent(pathname).slice(1).split(
        "/",
      );
      [packageName, packageVersion] = splitBy(name, "@");
      if (rest.length > 0) {
        subPath = "/" + rest.join("/");
      }
    }

    if (packageScope !== "" && !regexpNpmNaming.test(packageScope.slice(1))) {
      return err(`Invalid scope name '${packageScope}'`, 400);
    }

    if (packageName === "") {
      return err("Invalid path", 400);
    }

    if (!regexpNpmNaming.test(packageName)) {
      return err(`Invalid package name '${packageName}'`, 400);
    }

    const isTargetUrl = hasTargetSegment(subPath);

    let pkgId = packageName;
    if (packageScope) {
      pkgId = packageScope + "/" + packageName;
      if (gh) {
        // strip the leading `@`
        pkgId = pkgId.slice(1);
      }
    }

    // normalize package version
    if (packageVersion) {
      [packageVersion, extraQuery] = splitBy(packageVersion, "&");
      if (!gh) {
        if (
          packageVersion.startsWith("=") || packageVersion.startsWith("v")
        ) {
          packageVersion = packageVersion.slice(1);
        } else if (/^\d+$/.test(packageVersion)) {
          packageVersion = "~" + packageVersion;
        } else if (/^\d+.\d+$/.test(packageVersion)) {
          packageVersion = "~" + packageVersion;
        }
      }
    }

    // redirect to commit-ish version for GitHub packages
    if (
      gh && !(packageVersion && (
        regexpCommitish.test(packageVersion) ||
        regexpFullVersion.test(trimPrefix(packageVersion, "v"))
      ))
    ) {
      return ctx.withCache(() =>
        fetchOrigin(
          req,
          env,
          ctx,
          url.pathname + url.search,
          corsHeaders(),
        )
      );
    }

    // redirect to specific version
    let isCaretVersion = false;
    if (
      !gh && (!packageVersion ||
        (
          !regexpFullVersion.test(packageVersion) &&
          !(isCaretVersion = regexpCaretVersion.test(packageVersion))
        ))
    ) {
      return ctx.withCache(async () => {
        const headers = new Headers();
        if (env.NPM_TOKEN) {
          headers.set("Authorization", `Bearer ${env.NPM_TOKEN}`);
        }
        let registry = env.NPM_REGISTRY ?? defaultNpmRegistry;
        let pkgName = pkgId;
        if (pkgName.startsWith("@jsr/")) {
          registry = "https://npm.jsr.io";
        }
        const res = await fetch(new URL(pkgName, registry), { headers });
        if (!res.ok) {
          if (res.status === 404 || res.status === 401) {
            return errPkgNotFound(pkgName);
          }
          return new Response(res.body, {
            status: res.status,
            headers: corsHeaders(),
          });
        }
        const regInfo: PackageRegistryInfo = await res.json();
        let prefix = "/";
        if (hasExternalAllMarker) {
          prefix += "*";
        }
        if (pkgName.startsWith("@jsr/") && !isTargetUrl) {
          pkgName = "jsr/@" + pkgName.slice(5).replace("__", "/");
        }
        const eq = extraQuery ? "&" + extraQuery : "";
        const distVersion = regInfo["dist-tags"]
          ?.[packageVersion || "latest"];
        if (distVersion) {
          const uri = `${prefix}${pkgName}@${distVersion}${eq}${subPath}${url.search}`;
          return redirect(new URL(uri, url), 302);
        }
        const versions = Object.keys(regInfo.versions ?? []).filter(validate)
          .sort(compareVersions);
        if (!packageVersion) {
          const latestVersion = versions.filter((v) => !v.includes("-")).pop() ?? versions.pop();
          if (latestVersion) {
            const uri = `${prefix}${pkgName}@${latestVersion}${eq}${subPath}${url.search}`;
            return redirect(new URL(uri, url), 302);
          }
        }
        try {
          const arr = packageVersion.includes("-") ? versions : versions.filter((v) => !v.includes("-"));
          for (let i = arr.length - 1; i >= 0; i--) {
            const v = arr[i];
            if (satisfies(v, packageVersion)) {
              const uri = `${prefix}${pkgName}@${v}${eq}${subPath}${url.search}`;
              return redirect(new URL(uri, url), 302);
            }
          }
        } catch (_) {
          // error of `satisfies` function
          return err(`Invalid package version '${packageVersion}'`);
        }
        return err("Could not get the package version");
      });
    }

    // redirect `/@types/PKG` to `.d.ts` files
    if (pkgId.startsWith("@types/") && (subPath === "" || !isDtsFile(subPath))) {
      return ctx.withCache(async () => {
        let p = pathname;
        if (subPath !== "") {
          p += "~.d.ts";
        } else {
          const headers = new Headers();
          if (env.NPM_TOKEN) {
            headers.set("Authorization", `Bearer ${env.NPM_TOKEN}`);
          }
          const res = await fetch(
            new URL(pkgId, env.NPM_REGISTRY ?? defaultNpmRegistry),
            { headers },
          );
          if (!res.ok) {
            if (res.status === 404 || res.status === 401) {
              return errPkgNotFound(pkgId);
            }
            return new Response(res.body, { status: res.status, headers });
          }
          const pkgJson: PackageInfo = await res.json();
          p += "/" + (pkgJson.types || pkgJson.typings || pkgJson.main || "index.d.ts");
        }
        return redirect(new URL(p, url), 301);
      });
    }

    // redirect to main css for CSS packages
    let css: string | undefined;
    if (!gh && (css = cssPackages[pkgId]) && subPath === "") {
      return redirect(new URL(`/${pkgId}@${packageVersion}/${css}`, url), 301);
    }

    // redirect to real package css file: `/PKG?css` -> `/v100/PKG/es2022/pkg.css`
    if (url.searchParams.has("css") && subPath === "") {
      let prefix = "";
      if (gh) {
        prefix += "/gh";
      }
      let target = url.searchParams.get("target");
      if (!target || !targets.has(target)) {
        target = getBuildTargetFromUA(ua);
      }
      return redirect(
        new URL(
          `${prefix}/${pkgId}@${packageVersion}/${target}/${packageName}.css`,
          url,
        ),
        301,
      );
    }

    // redirect to real wasm file: `/PKG/es2022/foo.wasm` -> `PKG/foo.wasm`
    if (isTargetUrl && (subPath.endsWith(".wasm") || subPath.endsWith(".json"))) {
      return ctx.withCache(() => {
        return fetchOrigin(req, env, ctx, url.pathname, corsHeaders());
      });
    }

    // if it's npm asset file
    if (subPath !== "") {
      const ext = splitBy(subPath, ".", true)[1];
      // use origin server response for `*.wasm?module`
      if (ext === "wasm" && url.searchParams.has("module")) {
        return ctx.withCache(() => {
          return fetchOrigin(
            req,
            env,
            ctx,
            url.pathname + "?module",
            corsHeaders(),
          );
        });
      }
      if (assetsExts.has(ext)) {
        return ctx.withCache(() => {
          const prefix = gh ? "/gh" : "";
          const pathname = `${prefix}/${pkgId}@${packageVersion}${subPath}`;
          return fetchOriginWithR2Cache(req, ctx, env, pathname);
        });
      }
    }

    // apply extraQuery
    if (extraQuery) {
      const params = new URLSearchParams(extraQuery);
      params.forEach((val, key) => {
        url.searchParams.set(key, val);
      });
    }
    if (url.hostname === "raw.esm.sh") {
      url.searchParams.set("raw", "");
    }

    let prefix = "";
    if (gh) {
      prefix += "/gh";
    }

    if (isDtsFile(subPath) || isTargetUrl) {
      return ctx.withCache(() => {
        const pathname = `${prefix}/${pkgId}@${packageVersion}${subPath}`;
        return fetchOriginWithKVCache(req, env, ctx, pathname, undefined, true);
      });
    }

    return ctx.withCache(async () => {
      const marker = hasExternalAllMarker ? "*" : "";
      const pathname = `${prefix}/${marker}${pkgId}@${packageVersion}${subPath}`;
      normalizeSearchParams(url.searchParams);
      if (isCaretVersion) {
        const res = await fetchOrigin(req, env, ctx, pathname + url.search, corsHeaders());
        return res;
      }
      return fetchOriginWithKVCache(req, env, ctx, pathname, url.search);
    }, { varyUA: true });
  }

  return { fetch: handler };
}

export {
  checkPreflight,
  corsHeaders,
  getBuildTargetFromUA,
  getContentType,
  hashText,
  redirect,
  targets,
  version,
  withESMWorker,
};

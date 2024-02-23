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
import { assetsExts, cssPackages, STABLE_VERSION, stableBuild, VERSION } from "./consts.ts";
import { getMimeType } from "./content_type.ts";
import {
  asKV,
  checkPreflight,
  copyHeaders,
  corsHeaders,
  err,
  errPkgNotFound,
  fixPkgVersion,
  hashText,
  hasTargetSegment,
  redirect,
  splitBy,
  trimPrefix,
} from "./utils.ts";

const regexpNpmNaming = /^[a-zA-Z0-9][\w\.\-]*$/;
const regexpFullVersion = /^\d+\.\d+\.\d+/;
const regexpCommitish = /^[a-f0-9]{10,}$/;
const regexpBuildVersion = /^(v\d+|stable)$/;
const regexpBuildVersionPrefix = /^\/(v\d+|stable)\//;

const version = `v${VERSION}`;
const defaultNpmRegistry = "https://registry.npmjs.org";
const defaultEsmServerOrigin = "https://esm.sh";
const immutableCache = "public, max-age=31536000, immutable";

const noopStorage: WorkerStorage = {
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
    "X-Esm-Worker-Version",
    "X-Forwarded-For",
    "X-Real-Ip",
    "X-Real-Origin",
  );
  if (!headers.has("X-Esm-Worker-Version")) {
    headers.set("X-Esm-Worker-Version", `v${VERSION}`);
  }
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
    if (
      res.status === 500 &&
      res.headers.get("Content-Type")?.startsWith("text/html")
    ) {
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
  const exposedHeaders = [];
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
  path: string,
  gzip?: boolean,
): Promise<Response> {
  let storeKey = path.slice(1);
  if (storeKey.startsWith("stable/")) {
    storeKey = `v${STABLE_VERSION}/` + storeKey.slice(7);
  } else if (storeKey.startsWith("+")) {
    storeKey = `modules/` + storeKey;
  }
  const headers = corsHeaders();
  const [pathname] = splitBy(path, "?", true);
  const R2 = Reflect.get(env, "R2") as R2Bucket | undefined ?? noopStorage;
  const KV = Reflect.get(env, "KV") as KVNamespace | undefined ?? asKV(R2);
  const fromWorker = req.headers.has("X-Real-Origin");
  const isModule = !(
    ctx.url.searchParams.has("raw") ||
    pathname.endsWith(".d.ts") ||
    pathname.endsWith(".d.mts") ||
    pathname.endsWith(".map")
  );

  if (!fromWorker) {
    if (isModule) {
      const { value, metadata } = await KV.getWithMetadata<HttpMetadata>(
        storeKey,
        "stream",
      );
      if (value && metadata) {
        let body = value as ReadableStream<Uint8Array>;
        if (gzip && typeof DecompressionStream !== "undefined") {
          body = body.pipeThrough(new DecompressionStream("gzip"));
        }
        headers.set("Content-Type", metadata.contentType);
        headers.set("Cache-Control", immutableCache);
        const exposedHeaders = [];
        if (metadata.buildId) {
          headers.set("X-Esm-Id", metadata.buildId);
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
        const contentType = obj.httpMetadata?.contentType ||
          getMimeType(path);
        headers.set("Content-Type", contentType);
        headers.set("Cache-Control", immutableCache);
        headers.set("X-Content-Source", "esm-worker");
        return new Response(obj.body, { headers });
      }
    }
  }

  const res = await fetchOrigin(req, env, ctx, path, headers);
  if (!res.ok) {
    return res;
  }

  const buffer = await res.arrayBuffer();
  const contentType = res.headers.get("Content-Type") || getMimeType(path);
  const cacheControl = res.headers.get("Cache-Control");
  const buildId = res.headers.get("X-Esm-Id");
  const dts = res.headers.get("X-TypeScript-Types");
  const exposedHeaders = [];

  headers.set("Content-Type", contentType);
  if (cacheControl) {
    headers.set("Cache-Control", cacheControl);
  }
  if (buildId) {
    headers.set("X-Esm-Id", buildId);
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
        value = new Response(value).body.pipeThrough<Uint8Array>(
          new CompressionStream("gzip"),
        );
      }
      ctx.waitUntil(KV.put(storeKey, value, {
        metadata: { contentType, dts, buildId },
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
  const r2 = Reflect.get(env, "R2") as R2Bucket | undefined ?? noopStorage;
  const ret = await r2.get(pathname.slice(1));
  if (ret) {
    resHeaders.set(
      "Content-Type",
      ret.httpMetadata?.contentType || getMimeType(pathname),
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
      getMimeType(pathname);
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

function withESMWorker(middleware?: Middleware) {
  const cachePromise = caches.open(`esm.sh/v${VERSION}`);

  async function handler(
    req: Request,
    env: Env,
    context: ExecutionContext,
  ): Promise<Response> {
    const resp = checkPreflight(req);
    if (resp) {
      return resp;
    }

    const url = new URL(req.url);
    const ua = req.headers.get("User-Agent");
    const cache = await cachePromise;
    const withCache: Context["withCache"] = async (fetcher, options) => {
      const isHeadMethod = req.method === "HEAD";
      const hasPinedTarget = targets.has(
        url.searchParams.get("target") ?? "",
      );
      const cacheKey = new URL(url);
      const varyUA = options?.varyUA && !hasPinedTarget &&
        !url.searchParams.has("raw");
      if (varyUA) {
        const target = getBuildTargetFromUA(ua);
        cacheKey.searchParams.set("target", target);
        //! don't delete this line, it used to ensure KV/R2 cache respecting different UA
        url.searchParams.set("target", target);
      }
      for (const key of ["x-real-origin", "x-esm-worker-version"]) {
        const value = req.headers.get(key);
        if (value) {
          cacheKey.searchParams.set(key, value);
        }
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
      if (varyUA) {
        const headers = new Headers(res.headers);
        headers.append("Vary", "User-Agent");
        res = new Response(res.body, { status: res.status, headers });
      }
      if (
        res.ok &&
        res.headers.get("Cache-Control")?.startsWith("public, max-age=")
      ) {
        context.waitUntil(cache.put(cacheKey, res.clone()));
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
      waitUntil: (p: Promise<any>) => context.waitUntil(p),
      withCache,
    };

    // for deno runtime
    if (Reflect.has(context, "connInfo")) {
      Object.assign(ctx, Reflect.get(context, "connInfo"));
    }

    let pathname = decodeURIComponent(url.pathname);

    // strip trailing slash
    if (pathname !== "/" && pathname.endsWith("/")) {
      pathname = pathname.slice(0, -1);
    }

    // return the CLI script
    if (
      ua === "undici" ||
      ua?.startsWith("Node/") ||
      ua?.startsWith("Deno/") ||
      ua?.startsWith("Bun/")
    ) {
      if (pathname === "/" || /^\/v\d+\/?$/.test(pathname)) {
        return ctx.withCache(
          () => fetchOrigin(req, env, ctx, pathname, corsHeaders()),
          { varyUA: true },
        );
      }
    }

    switch (pathname) {
      case "/error.js":
        return ctx.withCache(
          () =>
            fetchOrigin(
              req,
              env,
              ctx,
              pathname + url.search,
              corsHeaders(),
            ),
          { varyUA: true },
        );

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

    if (middleware) {
      const resp = await middleware(req, env, ctx);
      if (resp) {
        return resp;
      }
    }

    if (
      req.method === "POST" &&
      (pathname === "/build" || pathname === "/transform")
    ) {
      const input = await req.text();
      const key = "esm-build-" + await hashText(input);
      const storage = Reflect.get(env, "R2") as R2Bucket | undefined ??
        noopStorage;
      const KV = Reflect.get(env, "KV") as KVNamespace | undefined ??
        asKV(storage);
      const { value } = await KV.getWithMetadata(key, "stream");
      if (value) {
        const headers = corsHeaders();
        headers.set("content-type", "application/json");
        headers.set(
          "cache-control",
          "private, no-store, no-cache, must-revalidate",
        );
        headers.set("X-Content-Source", "esm-worker");
        return new Response(value, {
          headers,
        });
      }
      const res = await fetchOrigin(
        new Request(req.url, {
          method: "POST",
          headers: req.headers,
          body: input,
        }),
        env,
        ctx,
        `${pathname}${url.search}`,
        corsHeaders(),
      );
      if (res.status !== 200) {
        return res;
      }
      const body = await res.arrayBuffer();
      ctx.waitUntil(KV.put(key, body));
      return new Response(body, {
        status: res.status,
        headers: res.headers,
      });
    }

    if (req.method !== "GET" && req.method !== "HEAD") {
      return err("Method Not Allowed", 405);
    }

    // ban malicious requests
    if (
      pathname === "/favicon.ico" ||
      pathname.startsWith("/.") ||
      pathname.endsWith(".php")
    ) {
      return ctx.withCache(
        () =>
          new Response(null, {
            status: 404,
            headers: { "cache-control": immutableCache },
          }),
      );
    }

    // landing page or embed files
    if (pathname === "/" || pathname.startsWith("/embed/")) {
      return fetchOrigin(
        req,
        env,
        ctx,
        `${pathname}${url.search}`,
        corsHeaders(),
      );
    }

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

    // singleton build module
    if (pathname.startsWith("/+")) {
      return ctx.withCache(
        () => fetchOriginWithKVCache(req, env, ctx, pathname + url.search),
        { varyUA: true },
      );
    }

    let buildVersion = "v" + VERSION;

    // check pinned build version
    const hasBuildVerPrefix = regexpBuildVersionPrefix.test(pathname);
    const hasBuildVerQuery = !hasBuildVerPrefix &&
      regexpBuildVersion.test(url.searchParams.get("pin") ?? "");
    if (hasBuildVerPrefix) {
      const a = pathname.split("/");
      buildVersion = a[1];
      pathname = "/" + a.slice(2).join("/");
    } else if (hasBuildVerQuery) {
      buildVersion = url.searchParams.get("pin")!;
    }

    if (
      pathname === "/build" ||
      pathname === "/run" ||
      pathname === "/hot"
    ) {
      if (!hasBuildVerPrefix && !hasBuildVerQuery) {
        return redirect(
          new URL(`/${buildVersion}${pathname}${url.search}`, url),
          302,
        );
      }
      return ctx.withCache(() =>
        fetchOrigin(
          req,
          env,
          ctx,
          `/${buildVersion}${pathname}${url.search}`,
          corsHeaders(),
        ), { varyUA: true });
    }

    const gh = pathname.startsWith("/gh/");
    if (gh) {
      pathname = "/@" + pathname.slice(4);
    }

    // strip external all marker
    const hasExternalAllMarker = pathname.startsWith("/*");
    if (hasExternalAllMarker) {
      pathname = "/" + pathname.slice(2);
    }

    if (
      hasBuildVerPrefix && (
        pathname === "/node.ns.d.ts" ||
          pathname === "/hot.d.ts" || (
          pathname.startsWith("/node_") &&
          pathname.endsWith(".js") &&
          !pathname.slice(1).includes("/")
        )
      )
    ) {
      return ctx.withCache(() =>
        fetchOriginWithKVCache(
          req,
          env,
          ctx,
          `/${buildVersion}${pathname}${url.search}`,
          true,
        ), { varyUA: true });
    }

    let packageScope = "";
    let packageName = "";
    let packageVersion = "";
    let subPath = "";
    let extraQuery = "";

    if (pathname.startsWith("/@")) {
      const [scope, name, ...rest] = decodeURIComponent(pathname).slice(2)
        .split("/");
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

    const fromEsmsh = packageName.startsWith("~") &&
      regexpCommitish.test(packageName.slice(1));
    if (!fromEsmsh && !regexpNpmNaming.test(packageName)) {
      return err(`Invalid package name '${packageName}'`, 400);
    }

    let pkg = packageName;
    if (packageScope) {
      pkg = packageScope + "/" + packageName;
      if (gh) {
        // strip the leading `@`
        pkg = pkg.slice(1);
      }
    }

    // format package version
    if (packageVersion) {
      [packageVersion, extraQuery] = splitBy(packageVersion, "&");
      if (!gh) {
        if (
          packageVersion.startsWith("=") || packageVersion.startsWith("v")
        ) {
          packageVersion = packageVersion.slice(1);
        } else if (/^\d+$/.test(packageVersion)) {
          packageVersion = "^" + packageVersion;
        } else if (/^\d+.\d+$/.test(packageVersion)) {
          packageVersion = "~" + packageVersion;
        }
      }
    }

    if (fromEsmsh) {
      packageVersion = "0.0.0";
    }

    // redirect to commit-ish version
    if (
      gh && !(
        packageVersion && (
          regexpCommitish.test(packageVersion) ||
          regexpFullVersion.test(trimPrefix(packageVersion, "v"))
        )
      )
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
    if (!gh && !(packageVersion && regexpFullVersion.test(packageVersion))) {
      return ctx.withCache(async () => {
        const headers = new Headers();
        if (env.NPM_TOKEN) {
          headers.set("Authorization", `Bearer ${env.NPM_TOKEN}`);
        }
        const res = await fetch(
          new URL(pkg, env.NPM_REGISTRY ?? defaultNpmRegistry),
          { headers },
        );
        if (!res.ok) {
          if (res.status === 404 || res.status === 401) {
            return errPkgNotFound(pkg);
          }
          return new Response(res.body, {
            status: res.status,
            headers: corsHeaders(),
          });
        }
        const regInfo: PackageRegistryInfo = await res.json();
        let prefix = "/";
        if (hasBuildVerPrefix) {
          prefix += buildVersion + "/";
        }
        if (hasExternalAllMarker) {
          prefix += "*";
        }
        const eq = extraQuery ? "&" + extraQuery : "";
        const distVersion = regInfo["dist-tags"]
          ?.[packageVersion || "latest"];
        if (distVersion) {
          const uri = `${prefix}${pkg}@${fixPkgVersion(pkg, distVersion)}${eq}${subPath}${url.search}`;
          return redirect(new URL(uri, url), 302);
        }
        const versions = Object.keys(regInfo.versions ?? []).filter(validate)
          .sort(compareVersions);
        if (!packageVersion) {
          const latestVersion = versions.filter((v) => !v.includes("-")).pop() ?? versions.pop();
          if (latestVersion) {
            const uri = `${prefix}${pkg}@${fixPkgVersion(pkg, latestVersion)}${eq}${subPath}${url.search}`;
            return redirect(new URL(uri, url), 302);
          }
        }
        try {
          const arr = packageVersion.includes("-") ? versions : versions.filter((v) => !v.includes("-"));
          for (let i = arr.length - 1; i >= 0; i--) {
            const v = arr[i];
            if (satisfies(v, packageVersion)) {
              const uri = `${prefix}${pkg}@${fixPkgVersion(pkg, v)}${eq}${subPath}${url.search}`;
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
    if (
      pkg.startsWith("@types/") &&
      (subPath === "" || !subPath.endsWith(".d.ts"))
    ) {
      return ctx.withCache(async () => {
        let p = `/${buildVersion}${pathname}`;
        if (subPath !== "") {
          p += "~.d.ts";
        } else {
          const headers = new Headers();
          if (env.NPM_TOKEN) {
            headers.set("Authorization", `Bearer ${env.NPM_TOKEN}`);
          }
          const res = await fetch(
            new URL(pkg, env.NPM_REGISTRY ?? defaultNpmRegistry),
            { headers },
          );
          if (!res.ok) {
            if (res.status === 404 || res.status === 401) {
              return errPkgNotFound(pkg);
            }
            return new Response(res.body, { status: res.status, headers });
          }
          const pkgJson: PackageInfo = await res.json();
          p += "/" +
            (pkgJson.types || pkgJson.typings || pkgJson.main ||
              "index.d.ts");
        }
        return redirect(new URL(p, url), 301);
      });
    }

    // redirect to main css for CSS packages
    let css: string | undefined;
    if (!gh && (css = cssPackages[pkg]) && subPath === "") {
      return redirect(new URL(`/${pkg}@${packageVersion}/${css}`, url), 301);
    }

    // redirect to real package css file: `/PKG?css` -> `/v100/PKG/es2022/pkg.css`
    if (url.searchParams.has("css") && subPath === "") {
      let prefix = `/${buildVersion}`;
      if (gh) {
        prefix += "/gh";
      }
      let target = url.searchParams.get("target");
      if (!target || !targets.has(target)) {
        target = getBuildTargetFromUA(ua);
      }
      const pined = hasBuildVerPrefix || hasBuildVerQuery;
      return redirect(
        new URL(
          `${prefix}/${pkg}@${packageVersion}/${target}/${packageName}.css`,
          url,
        ),
        pined ? 301 : 302,
      );
    }

    // redirect to real wasm file: `/v100/PKG/es2022/foo.wasm` -> `PKG/foo.wasm`
    if (hasBuildVerPrefix && subPath.endsWith(".wasm")) {
      return ctx.withCache(() => {
        return fetchOrigin(req, env, ctx, url.pathname, corsHeaders());
      });
    }

    // npm assets
    if (!hasBuildVerPrefix && subPath !== "") {
      const ext = splitBy(subPath, ".", true)[1];
      // append missed build version prefix for dts
      // example: `/@types/react/index.d.ts` -> `/v100/@types/react/index.d.ts`
      if (subPath.endsWith(".d.ts") || subPath.endsWith(".d.mts")) {
        return redirect(new URL("/v" + VERSION + url.pathname, url), 301);
      }
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
          const pathname = `${prefix}/${pkg}@${packageVersion}${subPath}`;
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

    if (
      hasBuildVerPrefix &&
      (subPath.endsWith(".d.ts") || hasTargetSegment(subPath))
    ) {
      return ctx.withCache(() => {
        let prefix = `/${buildVersion}`;
        if (gh) {
          prefix += "/gh";
        }
        const path = `${prefix}/${pkg}@${packageVersion}${subPath}${url.search}`;
        return fetchOriginWithKVCache(req, env, ctx, path, true);
      });
    }

    return ctx.withCache(() => {
      let prefix = "";
      if (hasBuildVerPrefix) {
        prefix += `/${buildVersion}`;
      } else if (stableBuild.has(pkg)) {
        prefix += `/v${STABLE_VERSION}`;
      }
      if (gh) {
        prefix += "/gh";
      }
      const marker = hasExternalAllMarker ? "*" : "";
      const path = `${prefix}/${marker}${pkg}@${packageVersion}${subPath}${url.search}`;
      return fetchOriginWithKVCache(req, env, ctx, path);
    }, { varyUA: true });
  }

  return { fetch: handler };
}

export { checkPreflight, corsHeaders, getBuildTargetFromUA, hashText, redirect, targets, version, withESMWorker };

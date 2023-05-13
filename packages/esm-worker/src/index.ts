import type { ExecutionContext } from "@cloudflare/workers-types";
import type {
  Context,
  Middleware,
  PackageInfo,
  PackageRegistryInfo,
} from "../types/index.d.ts";
import { compareVersions, satisfies, validate } from "compare-versions";
import { getEsmaVersionFromUA, targets } from "./compat.ts";
import {
  assetsExts,
  cssPackages,
  STABLE_VERSION,
  stableBuild,
  VERSION,
} from "./consts.ts";
import {
  boolJoin,
  checkPreflight,
  copyHeaders,
  corsHeaders,
  fixPkgVersion,
  redirect,
  splitBy,
  stringifyUrlSearch,
  trimPrefix,
} from "./utils.ts";

const regexpNpmNaming = /^[a-zA-Z0-9][\w\.\-\_]*$/;
const regexpFullVersion = /^\d+\.\d+\.\d+/;
const regexpCommitish = /^[a-f0-9]{10,}$/;
const regexpBuildVersion = /^(v\d+|stable)$/;
const regexpBuildVersionPrefix = /^\/(v\d+|stable)\//;

class ESMWorker {
  middleware?: Middleware;
  cache?: Cache;

  constructor(middleware?: Middleware) {
    this.middleware = middleware;
  }

  async fetch(
    req: Request,
    env: Env,
    context: ExecutionContext,
  ): Promise<Response> {
    const resp = checkPreflight(req);
    if (resp) {
      return resp;
    }

    const url = new URL(req.url);
    const cache = this.cache ?? (this.cache = await caches.open(`v${VERSION}`));
    const withCache = async (fetcher: () => Promise<Response> | Response) => {
      // check if cache hits
      let res = await cache.match(url);
      if (res) {
        return res;
      }
      res = await fetcher();
      if (res.headers.get("Cache-Control")?.startsWith("public, max-age=")) {
        context.waitUntil(cache.put(url, res.clone()));
      }
      return res;
    };
    const ctx: Context = {
      cache,
      env,
      url,
      data: {},
      waitUntil: (p) => context.waitUntil(p),
      withCache,
    };

    let pathname = decodeURIComponent(url.pathname);
    let buildVersion = "v" + VERSION;

    switch (pathname) {
      case "/build":
        if (req.method === "POST" || req.method === "PUT") {
          return fetchServerOrigin(
            req,
            ctx,
            `${pathname}${url.search}`,
            corsHeaders(),
          );
        }
      case "/error.js":
      case "/status.json":
        return fetchServerOrigin(
          req,
          ctx,
          `${pathname}${url.search}`,
          corsHeaders(),
        );
      case "/esma-target":
        return new Response(
          getEsmaVersionFromUA(req.headers.get("User-Agent")),
          { headers: corsHeaders() },
        );
      default:
        // ban malicious requests
        if (pathname.startsWith("/.") || pathname.endsWith(".php")) {
          return new Response("Not found", { status: 404 });
        }
    }

    // return deno CLI script
    if (
      req.headers.get("User-Agent")?.startsWith("Deno/") &&
      (pathname === "/" || /^\/v\d+\/?$/.test(pathname))
    ) {
      return fetchServerOrigin(
        req,
        ctx,
        `${pathname}${url.search}`,
        corsHeaders(),
      );
    }

    if (this.middleware) {
      const resp = await this.middleware(req, ctx);
      if (resp) {
        return resp;
      }
    }

    if (pathname === "/" || pathname.startsWith("/embed/")) {
      return fetchServerOrigin(
        req,
        ctx,
        `${pathname}${url.search}`,
        corsHeaders(),
      );
    }

    // fix `/jsx-runtime` path in query
    if (
      url.search.endsWith("/jsx-runtime") ||
      url.search.endsWith("/jsx-dev-runtime")
    ) {
      const [q, jsx] = splitBy(url.search, "/", true);
      pathname = pathname + "/" + jsx;
      url.search = q;
      url.pathname = pathname;
    }

    // strip build version prefix
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

    if (pathname === "/build") {
      if (!hasBuildVerPrefix && !hasBuildVerQuery) {
        const headers = corsHeaders();
        headers.set("Cache-Control", "public, max-age=86400");
        return redirect(
          new URL(`/${buildVersion}/build`, url),
          302,
          headers,
        );
      }
      return ctx.withCache(() =>
        fetchServerOrigin(
          req,
          ctx,
          url.pathname + url.search,
          corsHeaders(),
        )
      );
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

    // strip loc
    if (/:\d+:\d+$/.test(pathname)) {
      pathname = splitBy(pathname, ":")[0];
    }

    // construct the headers with CORS headers
    const headers = corsHeaders();

    if (
      hasBuildVerPrefix && (
        pathname === "/node.ns.d.ts" || (
          pathname.startsWith("/node_") &&
          pathname.endsWith(".js") &&
          pathname.slice(1).indexOf("/") === -1
        )
      )
    ) {
      // for old(deleted) ployfill
      if (pathname === "/node_buffer.js") {
        headers.set("Cache-Control", "public, max-age=31536000, immutable");
        return redirect(
          new URL(`/${buildVersion}/buffer@6.0.3`, url),
          301,
          headers,
        );
      }
      return ctx.withCache(() =>
        fetchESM(req, ctx, headers, `/${buildVersion}${pathname}`, {
          gzip: true,
        })
      );
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
      const [name, ...rest] = decodeURIComponent(pathname).slice(1).split("/");
      [packageName, packageVersion] = splitBy(name, "@");
      if (rest.length > 0) {
        subPath = "/" + rest.join("/");
      }
    }

    if (packageScope !== "" && !regexpNpmNaming.test(packageScope.slice(1))) {
      return new Response(`Invalid scope name '${packageScope}'`, {
        status: 400,
        headers,
      });
    }

    if (packageName === "") {
      return new Response("Invalid path", { status: 400, headers });
    }

    const fromEsmsh = packageName.startsWith("~") &&
      regexpCommitish.test(packageName.slice(1));
    if (!fromEsmsh && !regexpNpmNaming.test(packageName)) {
      return new Response(`Invalid package name '${packageName}'`, {
        status: 400,
        headers,
      });
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
        if (packageVersion.startsWith("=") || packageVersion.startsWith("v")) {
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
      return ctx.withCache(async () => {
        return fetchServerOrigin(req, ctx, url.pathname + url.search, headers);
      });
    }

    // redirect to specific version
    if (!gh && !(packageVersion && regexpFullVersion.test(packageVersion))) {
      return ctx.withCache(async () => {
        const headers = new Headers();
        if (env.NPM_TOKEN) {
          headers.set("Authorization", `Bearer ${env.NPM_TOKEN}`);
        }
        const res = await fetch(`${env.NPM_REGISTRY}${pkg}`, { headers });
        if (!res.ok) {
          if (res.status === 404 || res.status === 401) {
            return pkgNotFound(pkg, headers);
          }
          return new Response(res.body, { status: res.status, headers });
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
        const distVersion = packageVersion
          ? regInfo["dist-tags"]?.[packageVersion]
          : undefined;
        if (distVersion) {
          const uri = `${prefix}${pkg}@${
            fixPkgVersion(pkg, distVersion)
          }${eq}${subPath}${url.search}`;
          headers.set("Cache-Control", "public, max-age=600");
          return redirect(new URL(uri, url), 302, headers);
        }
        const versions = Object.keys(regInfo.versions ?? []).filter(validate)
          .sort(compareVersions);
        if (!packageVersion) {
          const latestVersion = versions.filter((v) =>
            !v.includes("-")
          ).pop() ?? versions.pop();
          if (latestVersion) {
            const uri = `${prefix}${pkg}@${
              fixPkgVersion(pkg, latestVersion)
            }${eq}${subPath}${url.search}`;
            headers.set("Cache-Control", "public, max-age=600");
            return redirect(new URL(uri, url), 302, headers);
          }
        }
        try {
          const arr = packageVersion.includes("-")
            ? versions
            : versions.filter((v) => !v.includes("-"));
          for (let i = arr.length - 1; i >= 0; i--) {
            const v = arr[i];
            if (satisfies(v, packageVersion)) {
              const uri = `${prefix}${pkg}@${
                fixPkgVersion(pkg, v)
              }${eq}${subPath}${url.search}`;
              headers.set("Cache-Control", "public, max-age=600");
              return redirect(new URL(uri, url), 302, headers);
            }
          }
        } catch (error) {
          // error of `satisfies` function
          return new Response(`Invalid package version '${packageVersion}'`, {
            status: 500,
            headers,
          });
        }
        return new Response("Could not get the package version", {
          status: 500,
          headers,
        });
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
            `${env.NPM_REGISTRY}${pkg}/${packageVersion}`,
            { headers },
          );
          if (!res.ok) {
            if (res.status === 404 || res.status === 401) {
              return pkgNotFound(pkg, headers);
            }
            return new Response(res.body, { status: res.status, headers });
          }
          const pkgJson: PackageInfo = await res.json();
          p += "/" +
            (pkgJson.types || pkgJson.typings || pkgJson.main || "index.d.ts");
        }
        headers.set("Cache-Control", "public, max-age=31536000, immutable");
        return redirect(new URL(p, url), 301, headers);
      });
    }

    // redirect to main css for CSS packages
    let css: string | undefined;
    if (!gh && (css = cssPackages[pkg]) && subPath === "") {
      headers.set("Cache-Control", "public, max-age=31536000, immutable");
      return redirect(
        new URL(`/${pkg}@${packageVersion}/${css}`, url),
        301,
        headers,
      );
    }

    // redirect to real package css file: `/PKG?css` -> `/v100/PKG/es2022/pkg.css`
    if (url.searchParams.has("css") && subPath === "") {
      let prefix = `/${buildVersion}`;
      if (gh) {
        prefix += "/gh";
      }
      let target = url.searchParams.get("target");
      if (!target || !targets.has(target)) {
        const ua = req.headers.get("user-agent");
        target = getEsmaVersionFromUA(ua);
      }
      const pined = hasBuildVerPrefix || hasBuildVerQuery;
      if (pined) {
        headers.set("Cache-Control", "public, max-age=31536000, immutable");
      } else {
        headers.set("Cache-Control", "public, max-age=86400");
      }
      return redirect(
        new URL(
          `${prefix}/${pkg}@${packageVersion}/${target}/${packageName}.css`,
          url,
        ),
        pined ? 301 : 302,
        headers,
      );
    }

    // redirect to real wasm file: `/v100/PKG/es2022/foo.wasm` -> `PKG/foo.wasm`
    if (hasBuildVerPrefix && subPath.endsWith(".wasm")) {
      return ctx.withCache(() => {
        return fetchServerOrigin(req, ctx, url.pathname, headers);
      });
    }

    // npm assets
    if (!hasBuildVerPrefix && subPath !== "") {
      const ext = splitBy(subPath, ".", true)[1];
      // append missed build version prefix for dts
      // example: `/@types/react/index.d.ts` -> `/v100/@types/react/index.d.ts`
      if (subPath.endsWith(".d.ts") || subPath.endsWith(".d.mts")) {
        headers.set("Cache-Control", "public, max-age=31536000, immutable");
        return redirect(
          new URL("/v" + VERSION + url.pathname, url),
          301,
          headers,
        );
      }
      // use origin server response for `*.wasm?module`
      if (ext === "wasm" && url.searchParams.has("module")) {
        return ctx.withCache(() => {
          return fetchServerOrigin(req, ctx, url.pathname + "?module", headers);
        });
      }
      if (assetsExts.has(ext)) {
        return ctx.withCache(() => {
          const pathname = `${
            gh ? "/gh" : ""
          }/${pkg}@${packageVersion}${subPath}`;
          return fetchAsset(req, ctx, pathname, headers);
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

    const cacheKey = new URL(url);

    // check `target` for caching strategy
    const hasPinedTarget = url.searchParams.has("target")! &&
      targets.has(
        url.searchParams.get("target")!,
      );
    if (
      !hasPinedTarget &&
      !pathname.split("/").some((p) => targets.has(p)) &&
      !pathname.endsWith(".d.ts")
    ) {
      const target = getEsmaVersionFromUA(req.headers.get("user-agent"));
      cacheKey.searchParams.set("target", target);
      headers.set("Vary", "Origin, User-Agent");
    }

    // check if cache hits
    let res = await ctx.cache.match(cacheKey);
    if (res) {
      return res;
    }

    if (
      hasBuildVerPrefix &&
      (subPath.endsWith(".d.ts") || hasTargetSegment(subPath))
    ) {
      // fix "stable" module path
      if (
        stableBuild.has(pkg) &&
        !subPath.endsWith(".d.ts") &&
        (subPath.endsWith(`/${pkg}.js`) ||
          !url.pathname.startsWith("/stable/")) &&
        subPath.split("/").length === 3
      ) {
        const [_, target] = subPath.split("/");
        headers.set("Cache-Control", "public, max-age=31536000, immutable");
        return redirect(
          new URL(`/${pkg}@${packageVersion}?target=${target}`, url),
          301,
          headers,
        );
      }
      let prefix = `/${buildVersion}`;
      if (gh) {
        prefix += "/gh";
      }
      const path = `${prefix}/${pkg}@${packageVersion}${subPath}${url.search}`;
      res = await fetchESM(req, ctx, headers, path, { gzip: true });
    } else {
      const search = url.searchParams;
      const target = cacheKey.searchParams.get("target");
      if (target) {
        search.set("target", target.toLowerCase());
      }
      let prefix = "";
      if (hasBuildVerPrefix) {
        prefix += `/${buildVersion}`;
      } else if (stableBuild.has(pkg)) {
        prefix += `/v${STABLE_VERSION}`;
      }
      if (gh) {
        prefix += "/gh";
      }
      const path = `${prefix}/${
        hasExternalAllMarker ? "*" : ""
      }${pkg}@${packageVersion}${subPath}${stringifyUrlSearch(search)}`;
      res = await fetchESM(req, ctx, headers, path, { gzip: false });
    }

    if (res.headers.get("Cache-Control")?.startsWith("public, max-age=")) {
      ctx.waitUntil(ctx.cache.put(cacheKey, res.clone()));
    }

    return res;
  }
}

export default function worker(middleware?: Middleware): ESMWorker {
  return new ESMWorker(middleware);
}

async function fetchAsset(
  req: Request,
  ctx: Context,
  pathname: string,
  resHeaders: Headers,
) {
  const r2 = ctx.env.R2;
  const r2o = await r2.get(pathname.slice(1));
  if (r2o) {
    resHeaders.set("Content-Type", r2o.httpMetadata.contentType);
    resHeaders.set("Cache-Control", "public, max-age=31536000, immutable");
    resHeaders.set("X-Content-Source", "r2");
    return new Response(r2o.body as ReadableStream<Uint8Array>, {
      headers: resHeaders,
    });
  }

  const res = await fetchServerOrigin(req, ctx, pathname, resHeaders);
  if (res.ok) {
    const contentType = fixContentType(
      res.headers.get("content-type"),
      pathname,
    );
    const buffer = await res.arrayBuffer();
    await r2.put(pathname.slice(1), buffer.slice(0), {
      httpMetadata: { contentType },
    });
    resHeaders.set("Content-Type", contentType);
    resHeaders.set("Cache-Control", "public, max-age=31536000, immutable");
    return new Response(buffer.slice(0), { headers: resHeaders });
  }
  return res;
}

async function fetchESM(
  req: Request,
  ctx: Context,
  headers: Headers,
  path: string,
  options: { gzip: boolean },
): Promise<Response> {
  let storeKey = path.slice(1);
  if (storeKey.startsWith("stable/")) {
    storeKey = `v${STABLE_VERSION}/` + storeKey.slice(7);
  }
  const { R2, KV } = ctx.env;
  const [pathname] = splitBy(path, "?", true);
  const storage = pathname.endsWith(".d.ts") || pathname.endsWith(".map")
    ? "r2"
    : "workers-kv";
  if (storage === "r2") {
    const obj = await R2.get(storeKey);
    if (obj) {
      headers.set("Content-Type", obj.httpMetadata.contentType);
      headers.set("Cache-Control", "public, max-age=31536000, immutable");
      headers.set("X-Content-Source", storage);
      return new Response(
        req.method === "HEAD" ? null : obj.body as ReadableStream<Uint8Array>,
        { headers },
      );
    }
  } else {
    const { value, metadata } = await KV.getWithMetadata<
      { contentType: string; cacheControl: string; dts?: string }
    >(storeKey, "stream");
    if (value && metadata) {
      let body = value as ReadableStream<Uint8Array>;
      if (options.gzip) {
        body = body.pipeThrough(new DecompressionStream("gzip"));
      }
      headers.set("Content-Type", metadata.contentType);
      headers.set("Cache-Control", "public, max-age=31536000, immutable");
      if (metadata.dts) {
        headers.set("X-TypeScript-Types", metadata.dts);
        headers.set("Access-Control-Expose-Headers", "X-TypeScript-Types");
      }
      headers.set("X-Content-Source", storage);
      return new Response(req.method === "HEAD" ? null : body, { headers });
    }
  }

  const res = await fetchServerOrigin(req, ctx, path, headers);
  if (!res.ok) {
    return res;
  }

  const contentType = fixContentType(res.headers.get("Content-Type"), path);
  const cacheControl = res.headers.get("Cache-Control");
  const dts = res.headers.get("X-TypeScript-Types");

  headers.set("Content-Type", contentType);
  if (cacheControl) {
    headers.set("Cache-Control", cacheControl);
  }
  if (dts) {
    headers.set("X-TypeScript-Types", dts);
    headers.set("Access-Control-Expose-Headers", "X-TypeScript-Types");
  }

  // save to KV/R2 if immutable
  if (req.method === "GET" && cacheControl?.includes("immutable")) {
    if (storage === "r2") {
      const buffer = await res.arrayBuffer();
      await R2.put(storeKey, buffer.slice(0), {
        httpMetadata: { contentType },
      });
      return new Response(buffer.slice(0), { headers });
    }
    // seems `local` CF worker doesn't support `ReadableStream.tee` yet, so we need to use `arrayBuffer` here
    if (ctx.env.WORKER_ENV === "development") {
      const buf = await res.arrayBuffer();
      const value = options.gzip
        ? new Response(buf).body.pipeThrough(new CompressionStream("gzip"))
        : buf;
      await KV.put(storeKey, value as any, { metadata: { contentType, dts } });
      return new Response(buf, { headers });
    }
    const [body, bodyCopy] = res.body.tee();
    const value = options.gzip
      ? bodyCopy.pipeThrough(new CompressionStream("gzip"))
      : bodyCopy;
    await KV.put(storeKey, value as any, { metadata: { contentType, dts } });
    return new Response(body, { headers });
  }

  return new Response(req.method === "HEAD" ? null : res.body, { headers });
}

async function fetchServerOrigin(
  req: Request,
  ctx: Context,
  url: string,
  resHeaders: Headers,
): Promise<Response> {
  const headers = new Headers(req.headers);
  copyHeaders(
    headers,
    req.headers,
    "Accept-Encoding",
    "Content-Type",
    "Referer",
    "User-Agent",
    "X-Real-Ip",
    "X-Forwarded-For",
  );
  if (ctx.env.ESM_SERVER_AUTH_TOKEN) {
    headers.set("Authorization", `Bearer ${ctx.env.ESM_SERVER_AUTH_TOKEN}`);
  }
  const res = await fetch(new URL(url, ctx.env.ESM_SERVER_ORIGIN), {
    method: req.method,
    body: req.body,
    headers,
    redirect: "manual",
  });
  if (!res.ok) {
    // fix cache-control by status code
    if (res.headers.has("Cache-Control")) {
      resHeaders.set("Cache-Control", res.headers.get("Cache-Control")!);
    } else if (res.status === 301 || res.status === 400) {
      resHeaders.set("Cache-Control", "public, max-age=31536000, immutable");
    } else if (res.status === 302) {
      resHeaders.set("Cache-Control", "public, max-age=600");
    } else if (res.status === 404) {
      const message = await res.text();
      if (!/package .+ not found/.test(message)) {
        resHeaders.set(
          "Cache-Control",
          "public, max-age=31536000, immutable",
        );
      }
      return new Response(message, { status: 404, headers: resHeaders });
    } else if (res.status === 500) {
      resHeaders.set("Cache-Control", "public, max-age=60");
    }
    if (res.status === 301 || res.status === 302) {
      // await res.body?.cancel?.()
      return redirect(res.headers.get("Location")!, res.status, resHeaders);
    }
    if (
      res.status === 500 &&
      res.headers.get("Content-Type")?.startsWith("text/html")
    ) {
      // await res.body?.cancel?.();
      return new Response("Bad Gateway", { status: 502, headers: resHeaders });
    }
    return new Response(res.body, { status: res.status, headers: resHeaders });
  }
  copyHeaders(
    resHeaders,
    res.headers,
    "Cache-Control",
    "Content-Type",
    "Content-Length",
    "X-Typescript-Types",
  );
  if (resHeaders.has("X-Typescript-Types")) {
    resHeaders.set("Access-Control-Expose-Headers", "X-TypeScript-Types");
  }
  return new Response(res.body, { headers: resHeaders });
}

function hasTargetSegment(path: string) {
  const parts = path.slice(1).split("/");
  return parts.length >= 2 && parts.some((p) => targets.has(p));
}

function fixContentType(type: string | null, path: string) {
  const [pathname] = splitBy(path, "?", true);
  if (pathname.endsWith(".wasm")) {
    return "application/wasm";
  }
  const [t, charset] = splitBy(type ?? "", ";", true);
  if (
    (pathname.endsWith(".js") || pathname.endsWith(".mjs")) &&
    t !== "application/javascript"
  ) {
    return boolJoin(["application/javascript", charset], ";");
  }
  if (
    (pathname.endsWith(".ts") || pathname.endsWith(".mts")) &&
    t !== "application/typescript"
  ) {
    return boolJoin(["application/typescript", charset], ";");
  }
  if (pathname.endsWith(".map") && t !== "application/json") {
    return boolJoin(["application/json", charset], ";");
  }
  return type ?? "application/octet-stream";
}

function pkgNotFound(pkg: string, headers: Headers) {
  headers.set("Content-Type", "application/javascript; charset=utf-8");
  return new Response(
    [
      `/* esm.sh - error */`,
      `throw new Error("[esm.sh] " + "npm: package '${pkg}' not found");`,
      `export default null;`,
    ].join("\n"),
    { status: 404, headers },
  );
}

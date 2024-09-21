import type { Context, Middleware, PackageInfo, PackageRegistryInfo } from "../types/index.d.ts";
import { compareVersions, satisfies, validate } from "compare-versions";
import { getBuildTargetFromUA, targets } from "esm-compat";
import { assetsExts, cssPackages, VERSION } from "./consts.ts";
import { getContentType } from "./media_type.ts";
import { isDtsFile, isObject, normalizeSearchParams, redirect, splitBy, trimPrefix } from "./utils.ts";
import { copyHeaders, err, errPkgNotFound, getUrlOrigin, hasTargetSegment } from "./utils.ts";

const version = `v${VERSION}`;
const globalEtag = `W/"${version}"`;
const defaultEsmServerOrigin = "https://esm.sh";
const defaultNpmRegistry = "https://registry.npmjs.org";
const jsrNpmRegistry = "https://npm.jsr.io";
const ccImmutable = "public, max-age=31536000, immutable";

const regexpNpmNaming = /^[\w\-+.$!*~()]*$/;
const regexpFullVersion = /^\d+\.\d+\.\d+[\w\-.+]*$/;
const regexpCommitish = /^[a-f0-9]{7,}$/;
const regexpLegacyVersionPrefix = /^\/v\d+\//;
const regexpLegacyBuild = /^\/~[a-f0-9]{40}$/;
const regexpLocSuffix = /:\d+:\d+$/;

/** fetch data from the origin server */
async function fetchOrigin(req: Request, env: Env, ctx: Context, pathname: string, query?: string): Promise<Response> {
  const headers = new Headers();
  copyHeaders(
    headers,
    req.headers,
    "Content-Type",
    "Referer",
    "User-Agent",
    "If-None-Match",
    "X-Forwarded-For",
    "X-Real-Ip",
    "X-Real-Origin",
  );
  if (!headers.has("X-Real-Origin")) {
    headers.set("X-Real-Origin", ctx.url.origin);
  }
  if (env.ESM_SERVER_TOKEN) {
    headers.set("Authorization", `Bearer ${env.ESM_SERVER_TOKEN}`);
  }
  if (env.ZONE_ID) {
    headers.set("X-Zone-Id", env.ZONE_ID);
  }
  if (env.NPMRC) {
    headers.set("X-Npmrc", env.NPMRC);
  }
  const res = await fetch(
    new URL(pathname + (query ?? ""), env.ESM_SERVER_ORIGIN ?? defaultEsmServerOrigin),
    {
      method: req.method === "HEAD" ? "GET" : req.method,
      body: req.body,
      headers,
      redirect: "manual",
    },
  );
  if (!res.ok) {
    const { status, statusText } = res;
    const resHeaders = new Headers();
    copyHeaders(resHeaders, res.headers, "Cache-Control", "Content-Type");
    // redirects
    if (status === 301 || status === 302) {
      resHeaders.set("Location", res.headers.get("Location")!);
    }
    return new Response(res.body, { status, statusText, headers: resHeaders });
  }

  const resHeaders = new Headers();
  copyHeaders(
    resHeaders,
    res.headers,
    "Cache-Control",
    "Content-Type",
    "ETag",
    "X-Esm-Path",
    "X-TypeScript-Types",
  );
  const exposedHeaders: string[] = [];
  for (const key of ["X-Esm-Path", "X-TypeScript-Types"]) {
    if (resHeaders.has(key)) {
      exposedHeaders.push(key);
    }
  }
  if (exposedHeaders.length > 0) {
    resHeaders.set("Access-Control-Expose-Headers", exposedHeaders.join(", "));
  }
  return new Response(res.body, { headers: resHeaders });
}

/** fetch asset files like *.wasm, *.json, etc. */
async function fetchAsset(req: Request, ctx: Context, env: Env, pathname: string): Promise<Response> {
  const R2 = env.R2;
  const storeKey = pathname.slice(1);
  const ret = await R2?.get(storeKey);
  if (ret) {
    const headers = ctx.corsHeaders();
    headers.set("Content-Type", ret.httpMetadata?.contentType || getContentType(pathname));
    headers.set("Cache-Control", ccImmutable);
    headers.set("Content-Source", "R2");
    return new Response(ret.body, { headers });
  }

  const res = await fetchOrigin(req, env, ctx, pathname);
  if (!res.ok) {
    copyHeaders(res.headers, ctx.corsHeaders());
    return res;
  }

  const headers = ctx.corsHeaders(res.headers);
  const contentType = res.headers.get("content-type") || getContentType(pathname);
  headers.set("Content-Type", contentType);
  headers.set("Cache-Control", ccImmutable);

  if (R2) {
    const putOptions: R2PutOptions = { httpMetadata: { contentType } };
    if (typeof FixedLengthStream === "function" && res.body instanceof FixedLengthStream) {
      const [body1, body2] = res.body.tee();
      ctx.waitUntil(R2.put(storeKey, body1, putOptions));
      return new Response(body2, { headers });
    }
    const buffer = await res.arrayBuffer();
    ctx.waitUntil(R2.put(storeKey, buffer, putOptions));
    return new Response(buffer, { headers });
  }
  return new Response(res.body, { headers });
}

/** fetch build files like *.js, *.mjs, *.css, etc. */
async function fetchBuildDist(req: Request, env: Env, ctx: Context, pathname: string, query?: string): Promise<Response> {
  const R2 = env.R2;
  const isRaw = ctx.url.searchParams.has("raw");
  const isDts = isDtsFile(pathname);
  const isStatic = isRaw || isDts || pathname.endsWith(".mjs.map") || pathname.endsWith(".js.map");
  const isFromUpWorker = req.headers.has("X-Real-Origin");

  let uri = pathname;
  if (query) {
    uri += query;
  }
  let storeKey = uri.slice(1);
  if (isStatic) {
    // ignore query string for asset files
    storeKey = pathname.slice(1);
    // ignore the leading `*` for raw files
    if ((isRaw || isDts) && storeKey.startsWith("*")) {
      storeKey = storeKey.slice(1);
    }
  }
  if (env.ZONE_ID) {
    storeKey = env.ZONE_ID + "/" + storeKey;
  }

  if (!isFromUpWorker && R2) {
    const obj = await R2.get(storeKey);
    if (obj) {
      const contentType = obj.httpMetadata?.contentType || getContentType(pathname);
      if (isStatic) {
        const headers = ctx.corsHeaders();
        headers.set("Content-Type", contentType);
        headers.set("Cache-Control", ccImmutable);
        headers.set("Content-Source", "R2");
        return new Response(obj.body, { headers });
      } else {
        const { body, customMetadata } = obj;
        const headers = ctx.corsHeaders();
        headers.set("Content-Type", contentType);
        headers.set("Cache-Control", ccImmutable);
        const exposedHeaders: string[] = [];
        if (customMetadata?.esmPath) {
          headers.set("X-Esm-Path", customMetadata.esmPath);
          exposedHeaders.push("X-Esm-Path");
        }
        if (customMetadata?.dts) {
          headers.set("X-TypeScript-Types", customMetadata.dts);
          exposedHeaders.push("X-TypeScript-Types");
        }
        if (exposedHeaders.length > 0) {
          headers.set("Access-Control-Expose-Headers", exposedHeaders.join(", "));
        }
        headers.set("Content-Source", "R2");
        return new Response(body, { headers });
      }
    }
  }

  const res = await fetchOrigin(req, env, ctx, uri);
  if (!res.ok) {
    copyHeaders(res.headers, ctx.corsHeaders());
    return res;
  }

  const headers = ctx.corsHeaders(res.headers);
  const contentType = res.headers.get("Content-Type") || getContentType(pathname);
  const cacheControl = res.headers.get("Cache-Control");
  const esmPath = res.headers.get("X-Esm-Path") ?? undefined;
  const dts = res.headers.get("X-TypeScript-Types") ?? undefined;
  const exposedHeaders: string[] = [];

  headers.set("Content-Type", contentType);
  if (cacheControl) {
    headers.set("Cache-Control", cacheControl);
  }
  if (esmPath) {
    headers.set("X-Esm-Path", esmPath);
    exposedHeaders.push("X-Esm-Path");
  }
  if (dts) {
    headers.set("X-TypeScript-Types", dts);
    exposedHeaders.push("X-TypeScript-Types");
  }
  if (exposedHeaders.length > 0) {
    headers.set("Access-Control-Expose-Headers", exposedHeaders.join(", "));
  }

  // save the file to KV/R2 if the `cache-control` header is set to `public, max-age=31536000, immutable`
  if (!isFromUpWorker && R2 && cacheControl === ccImmutable) {
    const customMetadata: Record<string, string> = {};
    const putOptions: R2PutOptions = { httpMetadata: { contentType }, customMetadata };
    if (esmPath) {
      customMetadata.esmPath = esmPath;
    }
    if (dts) {
      customMetadata.dts = dts;
    }
    if (typeof FixedLengthStream === "function" && res.body instanceof FixedLengthStream) {
      const [body1, body2] = res.body.tee();
      ctx.waitUntil(R2.put(storeKey, body1, putOptions));
      return new Response(body2, { headers });
    }
    const buffer = await res.arrayBuffer();
    ctx.waitUntil(R2.put(storeKey, buffer, putOptions));
    return new Response(buffer, { headers });
  }

  return new Response(res.body, { headers });
}

function withESMWorker(middleware?: Middleware, cache: Cache = (caches as any).default) {
  let npmrc: Npmrc | null = null;
  let allowList: Set<string> | null = null;

  function corsHeaders(origin: string | null, headers?: Headers) {
    const h = new Headers(headers);
    if (allowList !== null) {
      if (!origin || !allowList.has(origin)) {
        return h;
      }
      h.set("Access-Control-Allow-Origin", origin);
      h.append("Vary", "Origin");
    } else {
      h.set("Access-Control-Allow-Origin", "*");
    }
    h.set("Access-Control-Allow-Methods", "HEAD, GET, POST");
    return h;
  }

  async function respondWith(req: Request, env: Env, ctx: Context): Promise<Response> {
    const h = req.headers;
    const url = ctx.url;

    let pathname = url.pathname;

    // ban malicious requests
    if (pathname.startsWith("/.") || pathname.endsWith(".php")) {
      return ctx.withCache(() =>
        new Response(null, {
          status: 404,
          headers: { "cache-control": ccImmutable },
        })
      );
    }

    switch (pathname) {
      case "/error.js":
        return ctx.withCache(async () => {
          const res = await fetchOrigin(req, env, ctx, pathname, url.search);
          copyHeaders(res.headers, ctx.corsHeaders());
          return res;
        });

      case "/status.json":
        const res = await fetchOrigin(req, env, ctx, pathname);
        copyHeaders(res.headers, ctx.corsHeaders());
        return res;
    }

    // built-in modules/scripts
    if (
      pathname === "/run" ||
      pathname === "/run-helper" ||
      pathname === "/tsx" ||
      (pathname.startsWith("/node/") && pathname.endsWith(".js"))
    ) {
      const isChunkjs = pathname.startsWith("/node/chunk-");
      if (!isChunkjs) {
        const ifNoneMatch = h.get("If-None-Match");
        if (ifNoneMatch === globalEtag) {
          const headers = ctx.corsHeaders();
          headers.set("Cache-Control", "public, max-age=86400");
          headers.set("Content-Type", getContentType(pathname));
          return new Response(null, { status: 304, headers });
        }
      }
      return ctx.withCache((target) => {
        let query = target ? "?target=" + target : undefined;
        if (isChunkjs) {
          return fetchBuildDist(req, env, ctx, pathname, query);
        }
        return fetchOrigin(req, env, ctx, pathname, query);
      }, {
        varyUA: true,
      });
    }

    if (middleware) {
      const resp = await middleware(req, env, ctx);
      if (resp) {
        return resp;
      }
    }

    if (req.method === "POST") {
      switch (pathname) {
        case "/transform": {
          const res = await fetchOrigin(req, env, ctx, pathname);
          copyHeaders(res.headers, ctx.corsHeaders());
          return res;
        }
        case "/purge": {
          const res = await fetchOrigin(req, env, ctx, pathname);
          const ret: { zoneId?: string; deletedFiles: string[] } = await res.json();
          const { zoneId, deletedFiles } = ret;
          if (deletedFiles && deletedFiles.length > 0) {
            const { R2 } = env;
            if (R2) {
              const keys = zoneId ? deletedFiles.map((name) => zoneId + "/" + name) : deletedFiles;
              await R2.delete(keys);
            }
          }
          return Response.json(ret, { headers: ctx.corsHeaders() });
        }
        default:
          return err("Not Found", ctx.corsHeaders(), 404);
      }
    }

    if (req.method !== "GET" && req.method !== "HEAD") {
      return err("Method Not Allowed", ctx.corsHeaders(), 405);
    }

    // return 404 for robots.txt
    if (pathname === "/robots.txt") {
      return err("Not Found", ctx.corsHeaders(), 404);
    }

    // use the default landing page/embedded files
    if (pathname === "/" || pathname === "/favicon.ico" || pathname.startsWith("/embed/")) {
      return fetchOrigin(req, env, ctx, pathname);
    }

    // if it's a singleton build module which is created by https://esm.sh/tsx
    if (pathname.startsWith("/+") && (pathname.endsWith(".mjs") || pathname.endsWith(".mjs.map"))) {
      return ctx.withCache(() => {
        return fetchBuildDist(req, env, ctx, pathname);
      });
    }

    // use legacy worker if the bild version is specified in the path or query
    if (env.LEGACY_WORKER) {
      if (
        pathname == "/build" ||
        pathname.startsWith("/stable/") ||
        (pathname.startsWith("/v") && regexpLegacyVersionPrefix.test(pathname)) ||
        (pathname.startsWith("/~") && regexpLegacyBuild.test(pathname)) ||
        url.searchParams.has("pin")
      ) {
        return env.LEGACY_WORKER.fetch(req);
      }
    }

    // strip trailing slash
    if (pathname !== "/" && pathname.endsWith("/")) {
      pathname = pathname.slice(0, -1);
    }

    // decode entries `%5E` -> `^`
    if (pathname.includes("%")) {
      pathname = decodeURI(pathname);
    }

    // fix `/jsx-runtime` suffix in query, normally it happens with import maps
    if (url.search.endsWith("/jsx-runtime") || url.search.endsWith("/jsx-dev-runtime")) {
      const [q, jsxRuntime] = splitBy(url.search, "/", true);
      pathname = pathname + "/" + jsxRuntime;
      url.pathname = pathname;
      url.search = q;
    }

    // strip loc
    if (pathname.includes(":") && regexpLocSuffix.test(pathname)) {
      pathname = splitBy(pathname, ":")[0];
    }

    // fix pathname for GitHub/jsr registry
    const gh = pathname.startsWith("/gh/");
    if (gh) {
      pathname = pathname.slice(3);
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

    let pScope = "";
    let pName = "";
    let pVersion = "";
    let subPath = "";
    let extraQuery = "";
    let isModuleFullPath = false;

    if (gh || pathname.startsWith("/@")) {
      const [, scope, name, ...rest] = pathname.split("/");
      pScope = scope;
      [pName, pVersion] = splitBy(name, "@");
      if (rest.length > 0) {
        subPath = "/" + rest.join("/");
        isModuleFullPath = hasTargetSegment(rest);
      }
    } else {
      const [, name, ...rest] = pathname.split("/");
      [pName, pVersion] = splitBy(name, "@");
      if (rest.length > 0) {
        subPath = "/" + rest.join("/");
        isModuleFullPath = hasTargetSegment(rest);
      }
    }

    if (pScope !== "" && !regexpNpmNaming.test(pScope.slice(1))) {
      return err(`Invalid scope name '${pScope}'`, ctx.corsHeaders(), 400);
    }

    if (pName === "") {
      return err("Invalid path", ctx.corsHeaders(), 400);
    }
    if (!regexpNpmNaming.test(pName) || pVersion.endsWith(".") || pVersion.endsWith("-")) {
      return err(`Invalid package name '${pName}'`, ctx.corsHeaders(), 400);
    }

    // hide source map files
    if (isModuleFullPath && subPath.endsWith(".map") && env.SOURCE_MAP === "off") {
      return err("Source map is disabled", ctx.corsHeaders(), 404);
    }

    // normalize package version
    if (pVersion) {
      [pVersion, extraQuery] = splitBy(pVersion, "&");
      if (!gh) {
        if (pVersion.startsWith("=") || pVersion.startsWith("v")) {
          pVersion = pVersion.slice(1);
        }
      }
      if (extraQuery) {
        const params = new URLSearchParams(extraQuery);
        params.forEach((val, key) => {
          url.searchParams.set(key, val);
        });
      }
    }
    if (pVersion && pVersion.endsWith(".")) {
      return err(`Invalid package version '${pVersion}'`, ctx.corsHeaders(), 400);
    }

    // redirect to commit-ish version for GitHub packages
    if (
      gh && !(pVersion && (
        regexpCommitish.test(pVersion) ||
        regexpFullVersion.test(trimPrefix(pVersion, "v"))
      ))
    ) {
      return ctx.withCache(async () => {
        const res = await fetchOrigin(req, env, ctx, url.pathname, url.search);
        copyHeaders(res.headers, ctx.corsHeaders());
        return res;
      });
    }

    const pkgName = (pScope ? pScope + "/" : "") + pName;
    const isFixedVersion = !!pVersion && regexpFullVersion.test(pVersion);

    if (!isFixedVersion && !gh) {
      const redirectToSepcificVersion = async (targetFromUA: string | null) => {
        const headers = new Headers();
        const npmrc = ctx.npmrc;
        let { registry, token, user, password } = npmrc;
        if (pkgName.startsWith("@")) {
          const [scope] = pkgName.split("/");
          const reg = npmrc.registries[scope];
          if (reg) {
            ({ registry, token, user, password } = reg);
          }
        }
        if (token) {
          headers.set("Authorization", "Bearer " + token);
        } else if (user && password) {
          headers.set("Authorization", "Basic " + btoa(`${user}:${password}`));
        }
        const res = await fetch(new URL(pkgName, registry), { headers });
        if (!res.ok) {
          if (res.status === 404 || res.status === 401) {
            return errPkgNotFound(pkgName, ctx.corsHeaders());
          }
          return new Response("Failed to get package info: " + await res.text(), {
            status: res.status,
            headers: ctx.corsHeaders(),
          });
        }
        const regInfo: PackageRegistryInfo = await res.json();
        let prefix = "/";
        if (hasExternalAllMarker) {
          prefix += "*";
        }
        if (pkgName.startsWith("@jsr/") && !isModuleFullPath) {
          prefix += "jsr/@" + pkgName.slice(5).replace("__", "/");
        } else {
          prefix += pkgName;
        }
        let q = url.search;
        if (targetFromUA && url.searchParams.get("target") === targetFromUA) {
          url.searchParams.delete("target");
          q = url.search;
        }
        const xq = extraQuery ? "&" + extraQuery : "";
        const distVersion = regInfo["dist-tags"]?.[pVersion || "latest"];
        if (distVersion) {
          return redirect(
            new URL(prefix + "@" + distVersion + xq + subPath + q, url),
            ctx.corsHeaders(),
          );
        }
        const versions = Object.keys(regInfo.versions ?? []).filter(validate).sort(compareVersions);
        if (!pVersion) {
          const latestVersion = versions.filter((v) => !v.includes("-")).pop() ?? versions.pop();
          if (latestVersion) {
            return redirect(
              new URL(prefix + "@" + latestVersion + xq + subPath + q, url),
              ctx.corsHeaders(),
            );
          }
        }
        try {
          const arr = pVersion.includes("-") ? versions : versions.filter((v) => !v.includes("-"));
          for (let i = arr.length - 1; i >= 0; i--) {
            const v = arr[i];
            if (satisfies(v, pVersion)) {
              return redirect(
                new URL(prefix + "@" + v + xq + subPath + q, url),
                ctx.corsHeaders(),
              );
            }
          }
        } catch (_) {
          // not a semver version
          return err(`Invalid package version '${pVersion}'`, ctx.corsHeaders(), 400);
        }
        return err("Could not get the package version", ctx.corsHeaders(), 404);
      };
      if (
        (
          !isModuleFullPath &&
          !(subPath !== "" && assetsExts.has(splitBy(subPath, ".", true)[1])) &&
          !isDtsFile(subPath) &&
          !url.searchParams.has("raw")
        )
      ) {
        return ctx.withCache(async (targetFromUA) => {
          if (targetFromUA === "denonext" || targetFromUA === "deno") {
            return redirectToSepcificVersion(targetFromUA);
          }
          const res = await fetchOrigin(req, env, ctx, url.pathname, url.search);
          copyHeaders(res.headers, ctx.corsHeaders());
          return res;
        }, {
          varyUA: true,
        });
      }
      return ctx.withCache(redirectToSepcificVersion);
    }

    // redirect `/@types/PKG` to it's main dts file
    if (pkgName.startsWith("@types/") && subPath === "") {
      return ctx.withCache(async () => {
        const res = await fetch(new URL(pkgName, defaultNpmRegistry));
        if (!res.ok) {
          if (res.status === 404 || res.status === 401) {
            return errPkgNotFound(pkgName, ctx.corsHeaders());
          }
          return new Response("Failed to get package info: " + await res.text(), {
            status: res.status,
            headers: ctx.corsHeaders(),
          });
        }
        const pkgJson: PackageInfo = await res.json();
        return redirect(
          new URL("/" + (pkgJson.types || pkgJson.typings || pkgJson.main || "index.d.ts"), url),
          ctx.corsHeaders(),
          301,
        );
      });
    }

    // redirect to main css for CSS packages
    let css: string | undefined;
    if (!gh && (css = cssPackages[pkgName]) && subPath === "") {
      return redirect(new URL(`/${pkgName}@${pVersion}/${css}`, url), ctx.corsHeaders(), 301);
    }

    // redirect to real package css file: `/PKG?css` -> `/v100/PKG/es2022/pkg.css`
    if (url.searchParams.has("css") && subPath === "") {
      let prefix = "";
      if (gh) {
        prefix += "/gh";
      }
      let target = url.searchParams.get("target");
      if (!target || !targets.has(target)) {
        target = getBuildTargetFromUA(h.get("User-Agent"));
      }
      return redirect(
        new URL(`${prefix}/${pkgName}@${pVersion}/${target}/${pName}.css`, url),
        ctx.corsHeaders(),
        301,
      );
    }

    // redirect to real wasm file: `/PKG/es2022/foo.wasm` -> `PKG/foo.wasm`
    if (isModuleFullPath && (subPath.endsWith(".wasm") || subPath.endsWith(".json"))) {
      return ctx.withCache(async () => {
        const res = await fetchOrigin(req, env, ctx, url.pathname);
        copyHeaders(res.headers, ctx.corsHeaders());
        return res;
      });
    }

    // assets files
    if (subPath !== "") {
      const ext = splitBy(subPath, ".", true)[1];
      // use origin server response for `*.wasm?module`
      if (ext === "wasm" && url.searchParams.has("module")) {
        return ctx.withCache(async () => {
          const res = await fetchOrigin(req, env, ctx, url.pathname, "?module");
          copyHeaders(res.headers, ctx.corsHeaders());
          return res;
        });
      }
      if (assetsExts.has(ext)) {
        return ctx.withCache(() => {
          const prefix = gh ? "/gh" : "";
          const pathname = `${prefix}/${pkgName}@${pVersion}${subPath}`;
          return fetchAsset(req, ctx, env, pathname);
        });
      }
    }

    let prefix = "";
    if (gh) {
      prefix += "/gh";
    }

    if (isModuleFullPath || isDtsFile(subPath)) {
      return ctx.withCache(() => {
        const pathname = `${prefix}/${pkgName}@${pVersion}${subPath}`;
        return fetchBuildDist(req, env, ctx, pathname);
      });
    }

    return ctx.withCache(async (target) => {
      const marker = hasExternalAllMarker ? "*" : "";
      const pathname = `${prefix}/${marker}${pkgName}@${pVersion}${subPath}`;
      const params = url.searchParams;
      normalizeSearchParams(params);
      if (target) {
        params.set("target", target);
      }
      return fetchBuildDist(req, env, ctx, pathname, "?" + params.toString());
    }, { varyUA: true });
  }

  return {
    fetch: (req: Request, env: Env, workerCtx: ExecutionContext): Response | Promise<Response> => {
      // parse env.ALLOW_LIST to a Set if it's defined
      if (allowList === null && env.ALLOW_LIST) {
        allowList = new Set(
          env.ALLOW_LIST.split(",").map((v) => v.trim()).filter(Boolean).map((v) => ["https://" + v, "http://" + v])
            .flat(),
        );
      }

      // handle preflight request
      if (req.method === "OPTIONS") {
        const headers = corsHeaders(req.headers.get("Origin"));
        if (!headers.has("Access-Control-Allow-Origin")) {
          return new Response(null, { status: 403 });
        }
        // cache preflight response for 24 hours
        headers.set("Access-Control-Max-Age", "86400");
        const h = req.headers.get("Access-Control-Request-Headers");
        if (h) {
          headers.set("Access-Control-Allow-Headers", h);
          headers.append("Vary", "Access-Control-Allow-Headers");
        }
        return new Response(null, { status: 204, headers });
      }

      // prepare npmrc config
      if (npmrc === null) {
        npmrc = {
          registry: env.NPM_REGISTRY ? getUrlOrigin(env.NPM_REGISTRY) : defaultNpmRegistry,
          registries: { "@jsr": { registry: jsrNpmRegistry } },
        };
        if (env.NPM_TOKEN) {
          npmrc.token = env.NPM_TOKEN;
        } else if (env.NPM_USER && env.NPM_PASSWORD) {
          npmrc.user = env.NPM_USER;
          npmrc.password = env.NPM_PASSWORD;
        }
        if (env.NPMRC) {
          try {
            const v: Npmrc = JSON.parse(env.NPMRC);
            if (isObject(v)) {
              npmrc = v;
              if (npmrc.registry) {
                npmrc.registry = getUrlOrigin(npmrc.registry);
              } else {
                npmrc.registry = defaultNpmRegistry;
              }
              if (!npmrc.registries) {
                npmrc.registries = {};
              }
              if (!npmrc.registries["@jsr"]) {
                npmrc.registries["@jsr"] = { registry: jsrNpmRegistry };
              }
              for (const key in npmrc.registries) {
                const reg = npmrc.registries[key];
                if (reg.registry) {
                  reg.registry = getUrlOrigin(reg.registry);
                }
              }
            }
          } catch {
            // ignore
          }
        }
      }

      // check if the request 'referer' against the allow list
      if (allowList !== null) {
        const referer = req.headers.get("Referer");
        if (referer) {
          if (!allowList.has(new URL(referer).origin)) {
            return new Response("Forbidden", { status: 403 });
          }
        }
      }

      const url = new URL(req.url);
      const ctx: Context = {
        ...workerCtx,
        cache,
        npmrc,
        url,
        corsHeaders: (header) => corsHeaders(req.headers.get("Origin"), header),
        withCache: async (fetcher, options) => {
          const { pathname, searchParams } = url;
          const cacheKey = new URL(url); // clone
          const isHeadMethod = req.method === "HEAD";
          const targetArg = searchParams.get("target");
          const hasPinedTarget = !!targetArg && targets.has(targetArg);
          const realOrigin = req.headers.get("X-Real-Origin");
          let targetFromUA: string | undefined;
          let res: Response | undefined;
          if (options?.varyUA && !hasPinedTarget && !isDtsFile(pathname) && !searchParams.has("raw")) {
            targetFromUA = getBuildTargetFromUA(req.headers.get("User-Agent"));
            cacheKey.searchParams.set("target", targetFromUA);
          }
          if (realOrigin) {
            cacheKey.searchParams.set("X-Origin", realOrigin);
          }
          if (env.ZONE_ID) {
            cacheKey.searchParams.set("X-Zone-Id", env.ZONE_ID);
          }
          res = await cache.match(cacheKey);
          if (res) {
            if (targetFromUA) {
              res.headers.append("Vary", "User-Agent");
            }
            if (isHeadMethod) {
              return new Response(null, { status: 204, headers: res.headers });
            }
            return res;
          }
          res = await fetcher(targetFromUA ?? (hasPinedTarget ? targetArg : null));
          if (targetFromUA) {
            res.headers.append("Vary", "User-Agent");
          }
          if (res.ok && res.headers.get("Cache-Control")?.startsWith("public, max-age=")) {
            workerCtx.waitUntil(cache.put(cacheKey, res.clone()));
          }
          if (isHeadMethod) {
            return new Response(null, { status: res.status, headers: res.headers });
          }
          return res;
        },
        next: () => null as unknown as Response,
      };

      // add `raw` search param to the url if the hostname is `raw.esm.sh`
      if (url.hostname === "raw.esm.sh") {
        url.searchParams.set("raw", "");
      }

      return respondWith(req, env, ctx).catch((e) => {
        if (env.R2) {
          // store the error to R2 storage
          workerCtx.waitUntil(env.R2.put(
            `errors/${new Date().toISOString().split("T")[0]}/${Date.now()}.log`,
            JSON.stringify({
              url: req.url,
              headers: Object.fromEntries(req.headers.entries()),
              message: e.message,
              stack: e.stack,
            }),
          ));
        }
        return err(e.message, ctx.corsHeaders());
      });
    },
  };
}

export { getBuildTargetFromUA, getContentType, targets, version, withESMWorker };

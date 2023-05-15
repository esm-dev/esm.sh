import { fixedPkgVersions } from "./consts.ts";

export function fixPkgVersion(pkg: string, version: string) {
  for (const [k, v] of Object.entries(fixedPkgVersions)) {
    if (`${pkg}@${version}`.startsWith(k)) {
      return v;
    }
  }
  return version;
}

export function trimPrefix(s: string, prefix: string): string {
  if (prefix !== "" && s.startsWith(prefix)) {
    return s.slice(prefix.length);
  }
  return s;
}

export function splitBy(
  s: string,
  searchString: string,
  fromLast = false,
): [string, string] {
  const i = fromLast ? s.lastIndexOf(searchString) : s.indexOf(searchString);
  if (i >= 0) {
    return [s.slice(0, i), s.slice(i + searchString.length)];
  }
  return [s, ""];
}

export function boolJoin(arr: unknown[], separator: string) {
  return arr.filter(Boolean).join(separator);
}

export function fixContentType(type: string | null, path: string) {
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

/** create redirect response. */
export function redirect(
  url: URL | string,
  status: 301 | 302,
  cacheMaxAge = 600,
) {
  const headers = corsHeaders();
  headers.set("Location", url.toString());
  if (status === 301) {
    headers.set("Cache-Control", "public, max-age=31536000, immutable");
  } else {
    headers.set(
      "Cache-Control",
      `public, max-age=${cacheMaxAge}`,
    );
  }
  return new Response(null, { status, headers });
}

export function err(message: string, status: number = 500) {
  return new Response(
    message,
    { status, headers: corsHeaders() },
  );
}

export function errPkgNotFound(pkg: string) {
  const headers = corsHeaders();
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

export function checkPreflight(req: Request): Response | undefined {
  if (req.method === "OPTIONS" && req.headers.has("Origin")) {
    const headers = new Headers({
      "Access-Control-Allow-Origin": "*",
      "Access-Control-Allow-Methods": req.headers.get(
        "Access-Control-Request-Method",
      )!,
      "Access-Control-Allow-Headers": req.headers.get(
        "Access-Control-Request-Headers",
      )!,
    });
    headers.append("Vary", "Origin");
    headers.append("Vary", "Access-Control-Request-Method");
    headers.append("Vary", "Access-Control-Request-Headers");
    return new Response(null, { headers });
  }
  return void 0;
}

export function corsHeaders() {
  return new Headers({
    "Access-Control-Allow-Origin": "*",
    "Access-Control-Allow-Methods": "*",
    "Vary": "Origin",
  });
}

export function copyHeaders(dst: Headers, src: Headers, ...keys: string[]) {
  for (const k of keys) {
    if (src.has(k)) {
      dst.set(k, src.get(k)!);
    }
  }
}

export function stringifyUrlSearch(params: URLSearchParams) {
  const s = params.toString();
  return s ? "?" + s : "";
}

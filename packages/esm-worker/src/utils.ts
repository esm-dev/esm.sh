// npm registry to fetch package info
export const npmRegistry = "https://registry.npmjs.org/";

// npm package name regex
export const npmNamingRegexp = /^[a-zA-Z0-9][\w\.\-\_]*$/;

// extensions allowed of a package
export const assetsExts = new Set([
  "jsx",
  "ts",
  "mts",
  "tsx",
  "wasm",
  "css",
  "less",
  "sass",
  "scss",
  "json",
  "xml",
  "yml",
  "yaml",
  "txt",
  "md",
  "html",
  "htm",
  "svg",
  "png",
  "jpg",
  "webp",
  "gif",
  "eot",
  "ttf",
  "otf",
  "woff",
  "woff2",
]);

// stable build for UI libraries like react, to make sure the runtime is single copy
export const stableBuild = new Set([
  "react",
  "preact",
  "solid-js",
  "svelte",
  "vue",
]);

// fixed package versions
const fixedPkgVersions = {
  "@types/react@17": "17.0.56",
  "@types/react@18": "18.0.33",
  "isomorphic-ws@4": "5.0.0",
};

// css packages
export const cssPackages = {
  "normalize.css": "normalize.css",
  "@unocss/reset": "tailwind.css",
  "reset-css": "reset.css",
};

// return fixed package version
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

/** create redirect response with custom headers. */
export function redirect(
  url: URL | string,
  code: number,
  headers: HeadersInit,
) {
  headers = headers instanceof Headers ? headers : new Headers(headers);
  headers.set("Location", url.toString());
  return new Response(null, { status: code, headers });
}

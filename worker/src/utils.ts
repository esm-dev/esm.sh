import { targets } from "esm-compat";

export function isObject(v: unknown): v is Record<string, unknown> {
  return v !== null && typeof v === "object" && !Array.isArray(v);
}

export function isDtsFile(path: string) {
  return path.endsWith(".d.ts") || path.endsWith(".d.mts");
}

export function hasTargetSegment(segments: string[]) {
  const len = segments.length;
  if (len < 2) {
    return false;
  }
  const s0 = segments[0];
  if (s0.startsWith("X-") && len > 2) {
    return targets.has(segments[1]);
  }
  return targets.has(s0);
}

export function trimPrefix(s: string, prefix: string): string {
  if (prefix !== "" && s.startsWith(prefix)) {
    return s.slice(prefix.length);
  }
  return s;
}

export function splitBy(s: string, searchString: string, fromLast = false): [string, string] {
  const i = fromLast ? s.lastIndexOf(searchString) : s.indexOf(searchString);
  if (i >= 0) {
    return [s.slice(0, i), s.slice(i + searchString.length)];
  }
  return [s, ""];
}

/** create redirect response. */
export function redirect(url: URL | string, headers: Headers, status: 301 | 302 = 302, cacheMaxAge = 3600) {
  headers.set("Location", url.toString());
  if (status === 301) {
    headers.set("Cache-Control", "public, max-age=31536000, immutable");
  } else {
    headers.set("Cache-Control", `public, max-age=${cacheMaxAge}`);
  }
  return new Response(null, { status, headers });
}

export function err(message: string, headers: Headers, status: number = 500) {
  return new Response(message, { status, headers });
}

export function errPkgNotFound(pkg: string, headers: Headers) {
  headers.set("Content-Type", "application/javascript; charset=utf-8");
  headers.set("Cache-Control", "private, no-cache, no-store, must-revalidate");
  return new Response(
    [
      `/* esm.sh - error */`,
      `throw new Error("[esm.sh] " + "npm: package '${pkg}' not found");`,
      `export default null;`,
    ].join("\n"),
    { status: 404, headers },
  );
}

export function copyHeaders(dst: Headers, src: Headers, ...keys: string[]) {
  if (keys.length === 0) {
    for (const [k, v] of src) {
      dst.set(k, v);
    }
    return;
  }
  for (const k of keys) {
    if (src.has(k)) {
      dst.set(k, src.get(k)!);
    }
  }
}

export function getUrlOrigin(url: string): string {
  return new URL(url).origin;
}

const allowedSearchParams = new Set([
  "alias",
  "bundle",
  "bundle-deps",
  "bundle-all",
  "conditions",
  "css",
  "deno-std",
  "deps",
  "dev",
  "exports",
  "external",
  "ignore-annotations",
  "ignore-require",
  "importer",
  "jsx-runtime",
  "keep-names",
  "name",
  "no-bundle",
  "no-check",
  "no-dts",
  "path",
  "raw",
  "standalone",
  "target",
  "type",
  "v",
  "worker",
]);

export function normalizeSearchParams(parmas: URLSearchParams) {
  if (parmas.size > 0) {
    for (const k of parmas.keys()) {
      if (!allowedSearchParams.has(k)) {
        parmas.delete(k);
      }
    }
    // remove 'target' if 'raw' is set
    if (parmas.has("raw") && parmas.has("target")) {
      parmas.delete("target");
    }
    parmas.sort();
  }
}

export async function hashText(s: string): Promise<string> {
  const buffer = await crypto.subtle.digest(
    "SHA-1",
    new TextEncoder().encode(s),
  );
  return Array.from(new Uint8Array(buffer)).map((b) => b.toString(16).padStart(2, "0")).join("");
}

export const targets = new Set([
  "es2015",
  "es2016",
  "es2017",
  "es2018",
  "es2019",
  "es2020",
  "es2021",
  "es2022",
  "es2023",
  "es2024",
  "esnext",
  "deno",
  "denonext",
  "node",
]);

const allowedQueryKeys = new Set([
  "alias",
  "bundle",
  "conditions",
  "css",
  "ctx",
  "deps",
  "dev",
  "exports",
  "external",
  "ignore-annotations",
  "ignore-require",
  "im",
  "importer",
  "jsx",
  "keep-names",
  "name",
  "no-dts",
  "path",
  "raw",
  "standalone",
  "svelte",
  "target",
  "type",
  "url",
  "v",
  "vue",
  "worker",
]);

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

export function getBuildTargetFromUA(ua: string | null): string {
  if (!ua) {
    return "es2022";
  }
  if (ua.startsWith("ES/")) {
    const t = "es" + ua.slice(3);
    if (targets.has(t)) {
      return t;
    }
  }
  if (ua.startsWith("Deno/")) {
    const v = ua.slice(5).split(".");
    if (v.length >= 3) {
      const version = v.map(Number) as [number, number, number];
      if (!versionLargeThan(version, [1, 33, 1])) {
        return "deno";
      }
    }
    return "denonext";
  }
  if (
    ua === "undici"
    || ua.startsWith("Node.js/")
    || ua.startsWith("Node/")
    || ua.startsWith("Bun/")
  ) {
    return "node";
  }
  return "es2022";
}

function versionLargeThan(v1: [number, number, number], v2: [number, number, number]) {
  return v1[0] > v2[0]
    || (v1[0] === v2[0] && v1[1] > v2[1])
    || (v1[0] === v2[0] && v1[1] === v2[1] && v1[2] > v2[2]);
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

export function normalizeSearchParams(parmas: URLSearchParams) {
  if (parmas.size > 0) {
    for (const k of parmas.keys()) {
      if (!allowedQueryKeys.has(k)) {
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

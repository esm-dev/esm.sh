import type { HttpMetadata, WorkerStorage, WorkerStorageKV } from "../types/index";
import { targets } from "esm-compat";

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

export function isDtsFile(path: string) {
  return path.endsWith(".d.ts") || path.endsWith(".d.mts");
}

export function mockKV(storage: R2Bucket | WorkerStorage): WorkerStorageKV {
  return globalThis.__MOCK_KV__ ?? (globalThis.__MOCK_KV__ = {
    async getWithMetadata(
      key: string,
      _options: { type: "stream"; cacheTtl?: number },
    ): Promise<{ value: ReadableStream | null; metadata: HttpMetadata | null }> {
      const ret = await storage.get(key);
      if (ret === null) {
        return { value: null, metadata: null };
      }
      return {
        value: ret.body,
        metadata: ret.customMetadata as HttpMetadata | undefined ?? null,
      };
    },
    async put(
      key: string,
      value: ArrayBuffer | Uint8Array | ReadableStream,
      options?: { expirationTtl?: number; metadata?: HttpMetadata },
    ): Promise<void> {
      await storage.put(key, value, { customMetadata: options?.metadata });
    },
  });
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
export function redirect(url: URL | string, status: 301 | 302, cacheMaxAge = 600) {
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

export function checkPreflight(req: Request): Response | void {
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

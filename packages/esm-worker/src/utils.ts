import type {
  HttpMetadata,
  WorkerStorage,
  WorkerStorageKV,
} from "../types/index.d.ts";
import { fixedPkgVersions } from "./consts.ts";

export function asKV(
  storage?: R2Bucket | WorkerStorage,
): WorkerStorageKV | undefined {
  if (!storage) {
    return undefined;
  }
  return globalThis.__AS_KV__ ?? (globalThis.__AS_KV__ = {
    async getWithMetadata(
      key: string,
      _type: "stream",
    ): Promise<
      { value: ReadableStream | null; metadata: HttpMetadata | null }
    > {
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
      options?: { metadata?: HttpMetadata },
    ): Promise<void> {
      await storage.put(key, value, { customMetadata: options?.metadata });
    },
  });
}

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

// copied from https://github.com/websockets/utf-8-validate/blob/master/fallback.js
export function isValidUTF8(buffer: ArrayBuffer): boolean {
  const len = buffer.byteLength;
  const view = new Uint8Array(buffer);
  let i = 0;
  while (i < len) {
    if ((view[i] & 0x80) === 0x00) { // 0xxxxxxx
      i++;
    } else if ((view[i] & 0xe0) === 0xc0) { // 110xxxxx 10xxxxxx
      if (
        i + 1 === len ||
        (view[i + 1] & 0xc0) !== 0x80 ||
        (view[i] & 0xfe) === 0xc0 // overlong
      ) {
        return false;
      }

      i += 2;
    } else if ((view[i] & 0xf0) === 0xe0) { // 1110xxxx 10xxxxxx 10xxxxxx
      if (
        i + 2 >= len ||
        (view[i + 1] & 0xc0) !== 0x80 ||
        (view[i + 2] & 0xc0) !== 0x80 ||
        view[i] === 0xe0 && (view[i + 1] & 0xe0) === 0x80 || // overlong
        view[i] === 0xed && (view[i + 1] & 0xe0) === 0xa0 // surrogate (U+D800 - U+DFFF)
      ) {
        return false;
      }

      i += 3;
    } else if ((view[i] & 0xf8) === 0xf0) { // 11110xxx 10xxxxxx 10xxxxxx 10xxxxxx
      if (
        i + 3 >= len ||
        (view[i + 1] & 0xc0) !== 0x80 ||
        (view[i + 2] & 0xc0) !== 0x80 ||
        (view[i + 3] & 0xc0) !== 0x80 ||
        view[i] === 0xf0 && (view[i + 1] & 0xf0) === 0x80 || // overlong
        view[i] === 0xf4 && view[i + 1] > 0x8f || view[i] > 0xf4 // > U+10FFFF
      ) {
        return false;
      }

      i += 4;
    } else {
      return false;
    }
  }
  return true;
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

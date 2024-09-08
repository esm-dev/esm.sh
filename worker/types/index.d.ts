/// <reference types="@cloudflare/workers-types" />

declare global {
  interface Env {
    ESM_SERVER_ORIGIN?: string;
    ESM_SERVER_TOKEN?: string;
    ZONE_ID?: string;
    NPMRC?: string;
    NPM_REGISTRY?: string;
    NPM_TOKEN?: string;
    NPM_USER?: string;
    NPM_PASSWORD?: string;
    ALLOW_LIST?: string;
    SOURCE_MAP?: "on" | "off"; // default: "on"
    R2?: R2Bucket;
    LEGACY_WORKER?: { fetch: (req: Request) => Promise<Response> };
  }
  interface NpmRegistry {
    registry: string;
    token?: string;
    user?: string;
    password?: string;
  }
  interface Npmrc extends NpmRegistry {
    registries: Record<string, NpmRegistry>;
  }
}

// compatibility with Cloudflare R2
export interface WorkerStorage {
  get(key: string): Promise<
    {
      body: ReadableStream<Uint8Array>;
      httpMetadata?: R2HTTPMetadata;
      customMetadata?: Record<string, string>;
    } | null
  >;
  put(
    key: string,
    value: ArrayBufferLike | ArrayBuffer | ReadableStream,
    options?: {
      httpMetadata?: R2HTTPMetadata;
      customMetadata?: Record<string, string>;
    },
  ): Promise<void>;
}

export const version: string;
export const targets: Set<string>;
export const getBuildTargetFromUA: (ua: string | null) => string;
export const getContentType: (filename: string, defaultType?: string) => string;
export const hashText: (text: string) => Promise<string>;

export function withESMWorker(middleware?: Middleware, cache?: Cache): {
  fetch: (
    req: Request,
    env: Env,
    context: { waitUntil(promise: Promise<any>): void },
  ) => Promise<Response>;
};

export interface Context extends ExecutionContext {
  cache: Cache;
  npmrc: Npmrc;
  url: URL;
  corsHeaders(headers?: Headers): Headers;
  withCache(
    fetcher: (targetFromUA: string | null) => Promise<Response> | Response,
    options?: { varyUA?: boolean; varyReferer?: boolean },
  ): Promise<Response>;
  next: () => Response;
}

export interface Middleware {
  (req: Request, env: Env, ctx: Context): Response | Promise<Response>;
}

export type PackageInfo = {
  name: string;
  version: string;
  main?: string;
  types?: string;
  typings?: string;
};

export type PackageRegistryInfo = {
  name: string;
  versions: Record<string, any>;
  "dist-tags": Record<string, string>;
};

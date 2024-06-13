/// <reference types="@cloudflare/workers-types" />

declare global {
  interface Env {
    ESM_SERVER_ORIGIN?: string;
    ESM_SERVER_TOKEN?: string;
    NPMRC?: string;
    NPM_REGISTRY?: string;
    NPM_TOKEN?: string;
    NPM_USER?: string;
    NPM_PASSWORD?: string;
    ALLOW_LIST?: string;
    SOURCE_MAP?: "on" | "off";
    KV?: KVNamespace;
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

export type HttpMetadata = {
  contentType: string;
  esmPath?: string;
  dts?: string;
};

// compatibility with Cloudflare KV
export interface WorkerStorageKV {
  getWithMetadata(
    key: string,
    options: { type: "stream"; cacheTtl?: number },
  ): Promise<{ value: ReadableStream | null; metadata: HttpMetadata | null }>;
  put(
    key: string,
    value: string | ArrayBufferLike | ArrayBuffer | ReadableStream,
    options?: { expirationTtl?: number; metadata?: HttpMetadata | null },
  ): Promise<void>;
}

// compatibility with Cloudflare R2
export interface WorkerStorage {
  get(key: string): Promise<
    {
      body: ReadableStream<Uint8Array>;
      httpMetadata?: HttpMetadata;
      customMetadata?: Record<string, string>;
    } | null
  >;
  put(
    key: string,
    value: ArrayBufferLike | ArrayBuffer | ReadableStream,
    options?: {
      httpMetadata?: HttpMetadata;
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

export type Context = {
  cache: Cache;
  npmrc: Npmrc;
  url: URL;
  waitUntil(promise: Promise<any>): void;
  withCache(fetcher: () => Promise<Response> | Response, options?: { varyUA: boolean }): Promise<Response>;
  corsHeaders(headers?: Headers): Headers;
};

export interface Middleware {
  (req: Request, env: Env, ctx: Context): Response | void | Promise<Response | void>;
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

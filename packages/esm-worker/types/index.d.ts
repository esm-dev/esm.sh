declare global {
  interface Env {
    ESM_ORIGIN?: string;
    ESM_TOKEN?: string;
    NPM_REGISTRY?: string;
    NPM_TOKEN?: string;
    LEGACY_WORKER?: { fetch: (req: Request) => Promise<Response> };
  }
}

export type HttpMetadata = {
  contentType: string;
  esmId?: string;
  dts?: string;
};

// compatibility with Cloudflare KV
export interface WorkerStorageKV {
  getWithMetadata(
    key: string,
    type: "stream",
  ): Promise<
    { value: ReadableStream | null; metadata: HttpMetadata | null }
  >;
  put(
    key: string,
    value: string | ArrayBufferLike | ArrayBuffer | ReadableStream,
    options?: { metadata?: HttpMetadata | null },
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
export const getContentType: (filename: string, defaultType?: string) => string;
export const getBuildTargetFromUA: (ua: string | null) => string;
export const checkPreflight: (req: Request) => Response | undefined;
export const corsHeaders: () => Headers;
export const redirect: (url: URL | string, status: 301 | 302, cacheMaxAge?: number) => Response;
export const hashText: (text: string) => Promise<string>;

export function withESMWorker(middleware?: Middleware, cache?: Cache): {
  fetch: (
    req: Request,
    env: Env,
    context: { waitUntil(promise: Promise<any>): void },
  ) => Promise<Response>;
};

export type Context<Data = Record<string, any>> = {
  cache: Cache;
  data: Data;
  url: URL;
  waitUntil(promise: Promise<any>): void;
  withCache(
    fetcher: () => Promise<Response> | Response,
    options?: { varyUA: boolean },
  ): Promise<Response>;
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

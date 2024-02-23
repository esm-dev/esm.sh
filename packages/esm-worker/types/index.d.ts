declare global {
  interface Env {
    ESM_ORIGIN?: string;
    ESM_TOKEN?: string;
    NPM_REGISTRY?: string;
    NPM_TOKEN?: string;
  }
}

export type HttpMetadata = {
  contentType: string;
  buildId?: string;
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
export const getBuildTargetFromUA: (ua: string | null) => string;

export function withESMWorker(middleware?: Middleware): {
  fetch: (
    req: Request,
    env: Env,
    context: {
      connInfo?: Record<string, any>;
      waitUntil(promise: Promise<any>): void;
    },
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
  (
    req: Request,
    env: Env,
    ctx: Context,
  ): Response | void | Promise<Response | void>;
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

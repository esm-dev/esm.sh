import type {
  ExportedHandlerFetchHandler,
  KVNamespace,
  R2Bucket,
} from "@cloudflare/workers-types";

declare global {
  interface Env {
    WORKER_ENV: "development" | "production";
    KV: KVNamespace;
    R2: R2Bucket;
    NPM_REGISTRY: string;
    NPM_TOKEN?: string;
    ESM_SERVER_ORIGIN: string;
    ESM_SERVER_AUTH_TOKEN?: string;
  }
}

export default function (
  middleware?: Middleware,
): ExportedHandlerFetchHandler<Env, {}>;

export type Context<Data = Record<string, any>> = {
  cache: Cache;
  data: Data;
  env: Env;
  url: URL;
  waitUntil(promise: Promise<any>): void;
  withCache(fetch: () => Promise<Response> | Response): Promise<Response>;
};

export interface Middleware {
  (req: Request, ctx: Context): Promise<Response | undefined>;
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

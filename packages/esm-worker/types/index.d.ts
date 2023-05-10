import type {
  ExportedHandlerFetchHandler,
  KVNamespace,
  R2Bucket,
} from "@cloudflare/workers-types";

declare global {
  const __VERSION__: number;
  const __STABLE_VERSION__: number;
  interface Env {
    WORKER_ENV: "development" | "production";
    KV: KVNamespace;
    R2: R2Bucket;
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
  isDev: boolean;
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

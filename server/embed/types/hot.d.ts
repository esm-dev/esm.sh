export interface Plugin {
  name: string;
  setup: (hot: HotCore) => void;
}

export interface Loader {
  test: RegExp;
  load: (
    url: URL,
    source: string,
    options: LoadOptions,
  ) => Promise<LoaderOutput> | LoaderOutput;
  fetch?: (req: Request) => Promise<Response>;
}

export interface LoadOptions {
  isDev: boolean;
  importMap: ImportMap;
}

export type LoaderOutput = {
  code: string;
  contentType?: string;
  deps?: (DependencyDescriptor | string)[];
  map?: string;
};

export type DependencyDescriptor = {
  readonly specifier: string;
  readonly resolvedUrl?: string;
  readonly loc?: { start: number; end: number };
  readonly dynamic?: boolean;
};

export interface ImportMap {
  $support?: boolean;
  imports?: Record<string, string>;
  scopes?: Record<string, Record<string, string>>;
}

export interface FetchHandler {
  (req: Request): Response | Promise<Response>;
}

export interface URLTest {
  (url: URL, req: Request): boolean;
}

export interface VFSRecord {
  name: string;
  data: string | Uint8Array;
  meta?: VFSMeta;
}

export interface VFSMeta {
  [key: string]: any;
  checksum?: string;
  contentType?: string;
  deps?: (DependencyDescriptor | string)[];
}

export interface VFS {
  get(name: string): Promise<VFSRecord | null>;
  put(
    name: string,
    data: string | Uint8Array,
    meta?: VFSRecord["meta"],
  ): Promise<void>;
  delete(name: string): Promise<void>;
}

export interface HotCore {
  readonly basePath: string;
  readonly cache: Promise<Cache>;
  readonly customImports: Map<string, string>;
  readonly isDev: boolean;
  readonly vfs: VFS;
  fire(sw?: string): Promise<void>;
  listen(): void;
  onFetch(test: URLTest, handler: FetchHandler): this;
  onFire(handler: (reg: ServiceWorker) => void): this;
  onLoad(
    test: RegExp,
    load: Loader["load"],
    fetch?: Loader["fetch"],
    priority?: "eager",
  ): this;
  waitUntil(promise: Promise<any>): void;
}

export interface Hot extends HotCore, HotAPI {}

export default Hot;

export interface CallbackMap<T extends Function> {
  readonly map: Map<string, Set<T>>;
  add: (path: string, callback: T) => void;
  delete: (path: string, callback?: T) => void;
}

declare global {
  var __hot_hmr_modules: Set<string>;
  var __hot_hmr_callbacks: CallbackMap<(module: any) => void>;
  var __hot_hmr_disposes: CallbackMap<() => void>;
  interface HotAPI {}
}

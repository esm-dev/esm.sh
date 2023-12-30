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

export interface ContentMap {
  rendered?: Record<string, { value: any; expires?: number } | Promise<any>>;
  contents?: Record<string, ContentSource>;
}

export interface ContentSource {
  url?: string;
  method?: string;
  authorization?: string;
  headers?: [string, string][] | Record<string, string>;
  payload?: any;
  select?: string;
  timeout?: number;
  cacheTtl?: number;
  stream?: boolean;
}

export interface DevtoolsWidget {
  icon: string;
  component: string;
}

export interface DevtoolsWidgetFactory {
  (hot: Hot): DevtoolsWidget;
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
  get(name: string): Promise<VFSRecord | undefined>;
  put(
    name: string,
    data: string | Uint8Array,
    meta?: VFSRecord["meta"],
  ): Promise<string>;
  delete(name: string): Promise<void>;
}

export interface HotCore {
  readonly basePath: string;
  readonly cache: Cache;
  readonly contentMap: Required<ContentMap>;
  readonly importMap: Required<ImportMap>;
  readonly isDev: boolean;
  readonly vfs: VFS;
  fire(): Promise<void>;
  listen(swScript?: string): void;
  onFetch(test: URLTest, handler: FetchHandler): this;
  onFire(handler: (reg: ServiceWorker) => void): this;
  onLoad(
    test: RegExp,
    load: Loader["load"],
    fetch?: Loader["fetch"],
    priority?: "eager",
  ): this;
  waitUntil(promise: Promise<any>): void;
  use(...plugins: Plugin[]): this;
}

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

export interface Hot extends HotCore, HotAPI {}

export default Hot;

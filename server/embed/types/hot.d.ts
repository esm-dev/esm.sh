export interface Plugin {
  name: string;
  setup: (hot: HotCore) => void;
}

export interface ImportMap {
  $support?: boolean;
  imports?: Record<string, string>;
  scopes?: Record<string, Record<string, string>>;
}

export interface FetchHandler {
  (req: Request): Response | Promise<Response>;
}

export interface IncomingTest {
  (url: URL, method: string, headers: Headers): boolean;
}

export interface VFile {
  name: string;
  data: string | Uint8Array;
  meta?: VFileMeta;
}

export interface VFileMeta {
  [key: string]: any;
  contentType?: string;
}

export interface VFS {
  get(name: string): Promise<VFile | undefined>;
  put(
    name: string,
    data: string | Uint8Array,
    meta?: VFile["meta"],
  ): Promise<string>;
  delete(name: string): Promise<void>;
}

export interface HotCore {
  readonly cache: Cache;
  readonly importMap: Required<ImportMap>;
  readonly isDev: boolean;
  readonly vfs: VFS;
  fire(): Promise<void>;
  listen(swScript?: string): void;
  onFetch(test: IncomingTest | RegExp, handler: FetchHandler): this;
  onFire(handler: (reg: ServiceWorker) => void): this;
  waitUntil(promise: Promise<any>): void;
  use(...plugins: Plugin[]): this;
}

declare global {
  interface HotAPI {}
}

export interface Hot extends HotCore, HotAPI {}

export const hot: Hot;
export default hot;

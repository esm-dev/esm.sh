export interface Plugin {
  name: string;
  setup: (hot: HotCore) => void;
}

export interface IncomingTest {
  (url: URL, method: string, headers: Headers): boolean;
}

export interface FetchHandler {
  (req: Request): Response | Promise<Response>;
}

export interface ArchiveEntry {
  name: string;
  type: string;
  lastModified: number;
  offset: number;
  size: number;
}

export interface VFS {
  get(name: string): Promise<File | undefined>;
  put(file: File): Promise<string>;
  delete(name: string): Promise<void>;
}

export interface FireOptions {
  main?: string;
  swScript?: string;
  swUpdateViaCache?: ServiceWorkerUpdateViaCache;
}

export interface HotCore {
  readonly vfs: VFS;
  fire(options?: FireOptions): Promise<void>;
  listen(): void;
  onFire(handler: (reg: ServiceWorker) => void): this;
  waitUntil(...promises: readonly Promise<any>[]): this;
  use(...plugins: readonly Plugin[]): this;
}

declare global {
  interface HotAPI {}
}

export interface Hot extends HotCore, HotAPI {}

export const hot: Hot;
export default hot;

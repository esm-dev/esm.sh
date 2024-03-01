export interface Plugin {
  name: string;
  setup: (hot: HotCore) => void;
}

export interface ArchiveEntry {
  name: string;
  type: string;
  lastModified: number;
  offset: number;
  size: number;
}

export interface VFS {
  has(name: string): Promise<boolean>;
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
  use(...plugins: readonly Plugin[]): this;
  onFetch(handler: (event: FetchEvent) => void): this;
  onFire(handler: (reg: ServiceWorker) => void): this;
  onUpdateFound: () => void;
  fire(options?: FireOptions): Promise<void>;
  listen(): void;
  waitUntil(...promises: readonly Promise<any>[]): this;
}

declare global {
  interface HotAPI {}
}

export interface Hot extends HotCore, HotAPI {}

export const hot: Hot;
export default hot;

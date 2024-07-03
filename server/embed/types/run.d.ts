/// <reference lib="webworker" />

export interface ArchiveEntry {
  name: string;
  type: string;
  lastModified: number;
  offset: number;
  size: number;
}

export interface InstallOptions {
  main?: string;
  buildTarget?: `es20${15 | 16 | 17 | 18 | 19 | 20 | 21 | 22}`;
  swModule?: string;
  swScope?: string;
  swUpdateViaCache?: ServiceWorkerUpdateViaCache;
  onUpdateFound?: () => void;
}

export interface FireOptions {
  waitPromise?: Promise<void>;
  fetch?: (request: Request) => Promise<Response> | Response;
}

export function install(options?: InstallOptions): Promise<ServiceWorker>;
export function fire(options?: FireOptions): void;

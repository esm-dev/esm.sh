/// <reference lib="webworker" />

export interface ArchiveEntry {
  name: string;
  type: string;
  lastModified: number;
  offset: number;
  size: number;
}

export interface RunOptions {
  main?: string;
  buildTarget?: `es20${15 | 16 | 17 | 18 | 19 | 20 | 21 | 22}` | "esnext";
  swModule?: string;
  swScope?: string;
  swUpdateViaCache?: ServiceWorkerUpdateViaCache;
  onUpdateFound?: () => void;
}

export function run(options?: RunOptions): Promise<ServiceWorker>;
export default run;

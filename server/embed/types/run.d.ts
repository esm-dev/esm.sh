/// <reference lib="webworker" />

export interface VFile {
  url: string;
  content: Uint8Array;
  contentType?: string;
  lastModified?: number;
}

export interface RunOptions {
  main?: string;
  buildTarget?: `es20${15 | 16 | 17 | 18 | 19 | 20 | 21 | 22 | 23 | 24}` | "esnext";
  swModule?: string;
  swScope?: string;
  onUpdateFound?: () => void;
}

export function run(options?: RunOptions): Promise<ServiceWorker>;
export default run;

/// <reference lib="webworker" />

export interface VFile {
  url: string;
  content: Uint8Array;
  contentType?: string;
  lastModified?: number;
}

export interface RunOptions {
  main?: string;
  devSW?: string;
  sw?: string;
  swScope?: string;
  onUpdateFound?: () => void;
}

export function run(options?: RunOptions): Promise<ServiceWorker>;
export default run;

/// <reference lib="webworker" />

export interface RunOptions {
  main?: string;
  sw?: string | null;
  swScope?: string;
  onUpdateFound?: () => void;
}

export function run(options?: RunOptions): Promise<ServiceWorker>;
export default run;

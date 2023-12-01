export interface FsFile {
  size: number;
  lastModified: number | null;
  contentType: string;
  body: ReadableStream<Uint8Array>;
  close: () => Promise<void>;
}

export interface ServeOptions {
  cwd?: string;
  spa?: boolean | string | { index: string };
  plugins?: string[];
  watch?: boolean;
}

export function serveHost(
  options?: ServeOptions,
): (req: Request) => Promise<Response>;

export default serveHost;

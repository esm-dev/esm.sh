export interface FsFile {
  size: number;
  lastModified: number | null;
  contentType: string;
  body: ReadableStream<Uint8Array>;
  close: () => Promise<void>;
}

/** The options for `serveHost` */
export interface ServeOptions {
  /** The root path, default to current working directory. */
  root?: string;
  /** The fallback route, default is `index.html`. */
  fallback?: `${string}.html`;
}

/** Creates a fetch handler for serving hot applications. */
export function serveHost(
  options?: ServeOptions,
): (req: Request) => Promise<Response>;

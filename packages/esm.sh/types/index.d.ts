export interface FsFile {
  size: number;
  lastModified: number | null;
  contentType: string;
  body: ReadableStream<Uint8Array>;
  close: () => Promise<void>;
}

/** The options for `ESApp` */
export interface ESAppOptions {
  /** The root path, default to current working directory. */
  root?: string;
}

export interface ESApp {
  fetch: (req: Request) => Promise<Response>;
}

/** Creates a fetch handler for serving hot applications. */
export function createESApp(
  options?: ServeOptions,
): ESApp;

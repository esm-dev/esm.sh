export interface VFSOptions {
  scope?: string;
  version?: number;
  initial?: Record<string, string[] | string>;
}

export class VFS {
  list(): Promise<string[]>;
  readFile(name: string | URL): Promise<Uint8Array>;
  readFileWithVersion(name: string | URL): Promise<[Uint8Array, number]>;
  readTextFile(name: string | URL): Promise<string>;
  readTextFileWithVersion(name: string | URL): Promise<[string, number]>;
  writeFile(
    name: string | URL,
    content: string | Uint8Array,
    version?: number,
  ): Promise<void>;
}

export interface VFS {
  list(): Promise<string[]>;
  exists(path: string): Promise<boolean>;
  readFile(path: string): Promise<Uint8Array>;
  readTextFile(path: string): Promise<string>;
  writeFile(path: string, data: string | Uint8Array): Promise<void>;
}

export interface IDBFSOptions {
  scope?: string;
  version?: number;
  initial?: Record<string, string[] | string>;
}

export class IDBFS implements VFS {
  constructor(options: IDBFSOptions);
  list(): Promise<string[]>;
  exists(path: string): Promise<boolean>;
  readFile(path: string): Promise<Uint8Array>;
  readTextFile(path: string): Promise<string>;
  writeFile(path: string, data: string | Uint8Array): Promise<void>;
}

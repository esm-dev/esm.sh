import { editor } from "./monaco";

export interface VFSOptions {
  scope?: string;
  initial?: Record<string, string[] | string | Uint8Array>;
}

export class VFS {
  openModel(name: string | URL): Promise<editor.ITextModel>;
  exists(name: string | URL): Promise<boolean>;
  list(): Promise<string[]>;
  readFile(name: string | URL): Promise<Uint8Array>;
  readTextFile(name: string | URL): Promise<string>;
  writeFile(
    name: string | URL,
    content: string | Uint8Array,
    version?: number,
  ): Promise<void>;
  removeFile(name: string | URL): Promise<void>;
  watch(name: string | URL, handler: (evt: WatchEvent) => void): () => void;
}

interface WatchEvent {
  kind: "create" | "modify" | "remove";
  path: string;
}

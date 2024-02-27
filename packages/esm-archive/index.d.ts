export interface ArchiveEntry {
  name: string;
  type: string;
  lastModified: number;
  size: number;
}

export class Archive {
  readonly checksum: number;
  readonly entries: ArchiveEntry[];
  constructor(buffer: ArrayBufferLike);
  exists(name: string): boolean;
  openFile(name: string): File;
}

export function bundle(entries: FileList | File[]): Promise<Uint8Array>;

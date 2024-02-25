export interface ArchiveEntry {
  name: string;
  type: string;
  lastModified: number;
  size: number;
}

export class Archive {
  readonly checksum: number;
  readonly entries: ArchiveEntry[];
  static bundle(entries: File[]): Promise<Uint8Array>;
  constructor(buffer: ArrayBufferLike);
  readFile(name: string): File;
}

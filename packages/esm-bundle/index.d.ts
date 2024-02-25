export interface BundleEntry {
  name: string;
  type: string;
  lastModified: number;
  length: number;
  content: Uint8Array;
}

export class Bundle {
  readonly checksum: number;
  readonly entries: Omit<BundleEntry, "content">[];
  static bundle(entries: Omit<BundleEntry, "length">[]): Promise<Uint8Array>;
  constructor(buffer: ArrayBufferLike);
  readFile(name: string): File;
}

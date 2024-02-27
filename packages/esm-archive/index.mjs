import xxhash from "xxhash-wasm";

export async function bundle(entries) {
  if (!Archive.xxhash) {
    Archive.xxhash = xxhash();
  }
  if (Archive.xxhash instanceof Promise) {
    Archive.xxhash = await Archive.xxhash;
  }
  const encoder = new TextEncoder();
  const encode = (str) => encoder.encode(str);
  const length = 18 +
    Array.from(entries).reduce(
      (acc, { name, type, size }) => acc + 11 + encode(name).length + encode(type).length + size,
      0,
    );
  const buffer = new Uint8Array(length);
  const dv = new DataView(new ArrayBuffer(8));
  const h32 = Archive.xxhash.create32();
  dv.setUint32(0, length);
  buffer.set(encode("ESMARCHIVE"));
  buffer.set(new Uint8Array(dv.buffer), 10);
  let offset = 18;
  for (const entry of entries) {
    const name = encode(entry.name);
    const type = encode(entry.type);
    const content = new Uint8Array(await entry.arrayBuffer());
    if (name.length > 0xffff || type.length > 0xff) {
      throw new Error("entry name or type too long");
    }
    dv.setUint16(0, name.length);
    buffer.set(new Uint8Array(dv.buffer.slice(0, 2)), offset);
    offset += 2;
    buffer.set(name, offset);
    offset += name.length;
    buffer.set(new Uint8Array([type.length]), offset);
    offset += 1;
    buffer.set(type, offset);
    offset += type.length;
    dv.setUint32(0, Math.round((entry.lastModified ?? 0) / 1000)); // convert to seconds
    dv.setUint32(4, content.length);
    buffer.set(new Uint8Array(dv.buffer), offset);
    offset += 8;
    buffer.set(content, offset);
    offset += content.length;
    h32.update(name);
    h32.update(type);
    h32.update(new Uint8Array(dv.buffer));
    h32.update(content);
  }
  dv.setUint32(0, h32.digest());
  buffer.set(new Uint8Array(dv.buffer.slice(0, 4)), 14);
  return buffer;
}

export class Archive {
  #buffer;
  #checksum;
  #entries = {};

  static invalidFormat = new Error("Invalid esm archive format");

  constructor(buffer) {
    this.#buffer = buffer.buffer ?? buffer;
    this.#parse();
  }

  #parse() {
    const dv = new DataView(this.#buffer);
    const decoder = new TextDecoder();
    const readUint32 = (offset) => dv.getUint32(offset);
    const readString = (offset, length) => decoder.decode(new Uint8Array(this.#buffer, offset, length));
    if (this.#buffer.byteLength < 18 || readString(0, 10) !== "ESMARCHIVE") {
      throw Archive.invalidFormat;
    }
    const length = readUint32(10);
    if (length !== this.#buffer.byteLength) {
      throw Archive.invalidFormat;
    }
    this.#checksum = readUint32(14);
    let offset = 18;
    while (offset < dv.byteLength) {
      const nameLen = dv.getUint16(offset);
      offset += 2;
      const name = readString(offset, nameLen);
      offset += nameLen;
      const typeLen = dv.getUint8(offset);
      offset += 1;
      const type = readString(offset, typeLen);
      offset += typeLen;
      const lastModified = readUint32(offset) * 1000; // convert to ms
      offset += 4;
      const size = readUint32(offset);
      offset += 4;
      this.#entries[name] = { name, type, lastModified, offset, size };
      offset += size;
    }
  }

  get checksum() {
    return this.#checksum;
  }

  get entries() {
    return Object.values(this.#entries).map(({ offset, ...rest }) => rest);
  }

  exists(name) {
    return name in this.#entries;
  }

  openFile(name) {
    const info = this.#entries[name];
    return info ? new File([this.#buffer.slice(info.offset, info.offset + info.size)], info.name, info) : null;
  }
}

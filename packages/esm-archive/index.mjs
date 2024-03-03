import xxhash from "xxhash-wasm";

const xxhashPromise = xxhash();

export async function bundle(entries) {
  const encoder = new TextEncoder();
  const encode = (str) => encoder.encode(str);
  const length = 18 +
    Array.from(entries).reduce(
      (acc, { name, type, size }) => acc + 11 + encode(name).length + encode(type).length + size,
      0,
    );
  const u8Array = (init) => new Uint8Array(init);
  const buffer = u8Array(length);
  const h32 = (await xxhashPromise).create32();
  const dv = new DataView(new ArrayBuffer(8));
  const setUint32 = (i, n) => dv.setUint32(i, n);
  setUint32(0, length);
  buffer.set(encode("ESMARCHIVE"));
  buffer.set(u8Array(dv.buffer), 10);
  let offset = 18;
  for (const entry of entries) {
    const name = encode(entry.name);
    const type = encode(entry.type);
    const content = u8Array(await entry.arrayBuffer());
    if (name.length > 0xffff || type.length > 0xff) {
      throw new Error("entry name or type too long");
    }
    dv.setUint16(0, name.length);
    buffer.set(u8Array(dv.buffer.slice(0, 2)), offset);
    offset += 2;
    buffer.set(name, offset);
    offset += name.length;
    buffer.set(u8Array([type.length]), offset);
    offset += 1;
    buffer.set(type, offset);
    offset += type.length;
    setUint32(0, Math.round((entry.lastModified ?? 0) / 1000)); // convert to seconds
    setUint32(4, content.length);
    buffer.set(u8Array(dv.buffer), offset);
    offset += 8;
    buffer.set(content, offset);
    offset += content.length;
    for (const chunk of [name, type, u8Array(dv.buffer), content]) {
      h32.update(chunk);
    }
  }
  setUint32(0, h32.digest());
  buffer.set(u8Array(dv.buffer.slice(0, 4)), 14);
  return buffer;
}

export class Archive {
  _buf;
  _checksum;
  _entries;

  static invalidFormat = new Error("Invalid esm archive format");

  constructor(buffer) {
    this._buf = buffer.buffer ?? buffer;
    this._entries = {};
    this._parse();
  }

  _parse() {
    const dv = new DataView(this._buf);
    const decoder = new TextDecoder();
    const readUint32 = (offset) => dv.getUint32(offset);
    const readString = (offset, length) => decoder.decode(new Uint8Array(this._buf, offset, length));
    if (this._buf.byteLength < 18 || readString(0, 10) !== "ESMARCHIVE") {
      throw Archive.invalidFormat;
    }
    const length = readUint32(10);
    if (length !== this._buf.byteLength) {
      throw Archive.invalidFormat;
    }
    this._checksum = readUint32(14);
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
      this._entries[name] = { name, type, lastModified, offset, size };
      offset += size;
    }
  }

  get checksum() {
    return this._checksum;
  }

  get entries() {
    return Object.values(this._entries).map(({ offset, ...rest }) => rest);
  }

  exists(name) {
    return name in this._entries;
  }

  openFile(name) {
    const info = this._entries[name];
    return info ? new File([this._buf.slice(info.offset, info.offset + info.size)], info.name, info) : null;
  }
}

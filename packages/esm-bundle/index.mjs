import xxhash from "xxhash-wasm";

export class Bundle {
  #buffer;
  #checksum;
  #entries = {};

  static invalidBundle = new Error("Invalid bundle format");

  static async bundle(entries) {
    if (!Bundle.xxhash) {
      Bundle.xxhash = xxhash();
    }
    if (Bundle.xxhash instanceof Promise) {
      Bundle.xxhash = await Bundle.xxhash;
    }
    const encoder = new TextEncoder();
    const encode = (str) => encoder.encode(str);
    const length = 18 +
      entries.reduce(
        (acc, { name, type, content }) => acc + 11 + encode(name).length + encode(type).length + content.length,
        0,
      );
    const buffer = new Uint8Array(length);
    const dv = new DataView(new ArrayBuffer(8));
    const h32 = Bundle.xxhash.create32();
    dv.setUint32(0, length);
    buffer.set(encode("HOT_BUNDLE"));
    buffer.set(new Uint8Array(dv.buffer), 10);
    let offset = 18;
    for (const entry of entries) {
      const name = encode(entry.name);
      const type = encode(entry.type);
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
      dv.setUint32(4, entry.content.length);
      buffer.set(new Uint8Array(dv.buffer), offset);
      offset += 8;
      buffer.set(entry.content, offset);
      offset += entry.content.length;
      h32.update(name);
      h32.update(type);
      h32.update(new Uint8Array(dv.buffer));
      h32.update(entry.content);
    }
    dv.setUint32(0, h32.digest());
    buffer.set(new Uint8Array(dv.buffer.slice(0, 4)), 14);
    return buffer;
  }

  constructor(buffer) {
    this.#buffer = buffer.buffer ?? buffer;
    this.#parse();
  }

  get checksum() {
    return this.#checksum;
  }

  get entries() {
    return Object.values(this.#entries).map(({ offset, ...rest }) => rest);
  }

  #parse() {
    const dv = new DataView(this.#buffer);
    const decoder = new TextDecoder();
    const readString = (offset, length) => {
      return decoder.decode(new Uint8Array(this.#buffer, offset, length));
    };
    if (this.#buffer.byteLength < 18 || readString(0, 10) !== "HOT_BUNDLE") {
      throw Bundle.invalidBundle;
    }
    const length = dv.getUint32(10);
    if (length !== this.#buffer.byteLength) {
      throw Bundle.invalidBundle;
    }
    this.#checksum = dv.getUint32(14);
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
      const lastModified = dv.getUint32(offset) * 1000; // convert to ms
      offset += 4;
      const length = dv.getUint32(offset);
      offset += 4;
      this.#entries[name] = { name, type, lastModified, offset, length };
      offset += length;
    }
  }

  readFile(name) {
    const info = this.#entries[name];
    return info ? new File([this.#buffer.slice(info.offset, info.offset + info.length)], info.name, info) : null;
  }
}

const enc = new TextEncoder();
const dec = new TextDecoder();

export interface VFS {
  list(): Promise<string[]>;
  readFile(name: string | URL): Promise<Uint8Array>;
  readTextFile(name: string | URL): Promise<string>;
  writeFile(name: string | URL, data: string | Uint8Array): Promise<void>;
}

interface IDBFSItem {
  url: string;
  data: string | Uint8Array;
}

interface IDBFSOptions {
  scope?: string;
  version?: number;
  initial?: Record<string, string[] | string>;
}

export class IDBFS implements VFS {
  #db: Promise<IDBDatabase> | IDBDatabase;

  constructor(options: IDBFSOptions) {
    const req = indexedDB.open(
      "vfs:esm-monaco/" + (options.scope ?? "main"),
      options.version,
    );
    req.onupgradeneeded = async () => {
      const db = req.result;
      if (!db.objectStoreNames.contains("files")) {
        const store = db.createObjectStore("files", { keyPath: "url" });
        for (const [name, data] of Object.entries(options.initial ?? {})) {
          const url = new URL(name, "file:///");
          const item: IDBFSItem = {
            url: url.href,
            data: Array.isArray(data) ? data.join("\n") : data,
          };
          await waitIDBRequest(store.add(item));
        }
      }
    };
    this.#db = waitIDBRequest<IDBDatabase>(req).then((db) => this.#db = db);
  }

  async #begin(readonly = false) {
    let db = this.#db;
    if (db instanceof Promise) {
      db = await db;
    }
    return db.transaction("files", readonly ? "readonly" : "readwrite")
      .objectStore("files");
  }

  async list() {
    const db = await this.#begin(true);
    const list: string[] = [];
    const req = db.getAllKeys();
    return await waitIDBRequest<string[]>(req);
  }

  async #read(name: string | URL) {
    const url = new URL(name, "file:///");
    const db = await this.#begin(true);
    const ret = await waitIDBRequest<IDBFSItem>(
      db.get(url.href),
    );
    return ret.data;
  }

  async readFile(name: string | URL) {
    return toUint8Array(await this.#read(name));
  }

  async readTextFile(name: string | URL) {
    return toString(await this.#read(name));
  }

  async writeFile(name: string | URL, data: string | Uint8Array) {
    const url = new URL(name, "file:///");
    const db = await this.#begin();
    const item: IDBFSItem = { url: url.href, data };
    await waitIDBRequest(db.put(item));
  }
}

/** Convert string to Uint8Array. */
function toUint8Array(data: string | Uint8Array): Uint8Array {
  return typeof data === "string" ? enc.encode(data) : data;
}

/** Convert Uint8Array to string. */
function toString(data: string | Uint8Array) {
  return data instanceof Uint8Array ? dec.decode(data) : data;
}

/** wait for the given IDBRequest. */
function waitIDBRequest<T>(req: IDBRequest): Promise<T> {
  return new Promise((resolve, reject) => {
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

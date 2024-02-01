import * as monaco from "monaco-editor-core";

const enc = new TextEncoder();
const dec = new TextDecoder();
const idbVer = 1;

export interface VFSInterface {
  readonly ErrorNotFound: typeof ErrorNotFound;
  open(name: string | URL): Promise<monaco.editor.ITextModel>;
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
  watchFile?(
    name: string | URL,
    handler: (evt: { kind: string; path: string }) => void,
  ): () => void;
}

interface VFSItem {
  url: string;
  version: number;
  content: string | Uint8Array;
}

interface VFSOptions {
  scope?: string;
  initial?: Record<string, string[] | string | Uint8Array>;
}

export class VFS implements VFSInterface {
  #db: Promise<IDBDatabase> | IDBDatabase;
  #watchHandlers = new Map<
    string,
    Set<(evt: { kind: string; path: string }) => void>
  >();

  constructor(options: VFSOptions) {
    const req = indexedDB.open(
      "vfs:esm-monaco/" + (options.scope ?? "main"),
      idbVer,
    );
    req.onupgradeneeded = async () => {
      const db = req.result;
      if (!db.objectStoreNames.contains("files")) {
        const store = db.createObjectStore("files", { keyPath: "url" });
        for (const [name, data] of Object.entries(options.initial ?? {})) {
          const url = new URL(name, "file:///");
          const item: VFSItem = {
            url: url.href,
            version: 0,
            content: Array.isArray(data) && !(data instanceof Uint8Array)
              ? data.join("\n")
              : data,
          };
          await waitIDBRequest(store.add(item));
        }
      }
    };
    this.#db = waitIDBRequest<IDBDatabase>(req).then((db) => this.#db = db);
  }

  get ErrorNotFound() {
    return ErrorNotFound;
  }

  async #begin(readonly = false) {
    let db = this.#db;
    if (db instanceof Promise) {
      db = await db;
    }

    return db.transaction("files", readonly ? "readonly" : "readwrite")
      .objectStore("files");
  }

  async open(name: string | URL) {
    const url = new URL(name, "file:///");
    const uri = monaco.Uri.parse(url.href);
    const { content, version } = await this.#read(url);
    let model = monaco.editor.getModel(uri);
    if (model) {
      return model;
    }
    let writeTimer: number | null = null;
    model = monaco.editor.createModel(toString(content), undefined, uri);
    model.onDidChangeContent((e) => {
      if (writeTimer !== null) {
        return;
      }
      writeTimer = setTimeout(() => {
        writeTimer = null;
        this.writeFile(
          uri.path,
          model.getValue(),
          version + model.getVersionId(),
        );
      }, 500);
    });
    return model;
  }

  async exists(name: string | URL): Promise<boolean> {
    const url = new URL(name, "file:///").href;
    const db = await this.#begin(true);
    return waitIDBRequest<string>(db.getKey(url)).then((key) => !!key);
  }

  async list() {
    const db = await this.#begin(true);
    const req = db.getAllKeys();
    return await waitIDBRequest<string[]>(req);
  }

  async #read(name: string | URL) {
    const url = new URL(name, "file:///");
    const db = await this.#begin(true);
    const ret = await waitIDBRequest<VFSItem>(
      db.get(url.href),
    );
    if (!ret) {
      throw new ErrorNotFound(name);
    }
    return ret;
  }

  async readFile(name: string | URL) {
    const { content } = await this.#read(name);
    return toUint8Array(content);
  }

  async readTextFile(name: string | URL) {
    const { content } = await this.#read(name);
    return toString(content);
  }

  async writeFile(
    name: string | URL,
    content: string | Uint8Array,
    version?: number,
  ) {
    const { pathname, href } = new URL(name, "file:///");
    const db = await this.#begin();
    const item: VFSItem = { url: href, version: version ?? 0, content };
    await waitIDBRequest(db.put(item));
    const handlers = this.#watchHandlers.get(href);
    if (handlers) {
      for (const handler of handlers) {
        handler({ kind: "createOrModify", path: pathname });
      }
    }
  }

  async removeFile(name: string | URL): Promise<void> {
    const { pathname, href } = new URL(name, "file:///");
    const db = await this.#begin();
    await waitIDBRequest(db.delete(href));
    const handlers = this.#watchHandlers.get(href);
    if (handlers) {
      for (const handler of handlers) {
        handler({ kind: "remove", path: pathname });
      }
    }
  }

  watchFile(
    name: string | URL,
    handler: (evt: { kind: string; path: string }) => void,
  ): () => void {
    const url = new URL(name, "file:///").href;
    let handlers = this.#watchHandlers.get(url);
    if (!handlers) {
      handlers = new Set();
      this.#watchHandlers.set(url, handlers);
    }
    handlers.add(handler);
    return () => {
      handlers!.delete(handler);
    };
  }
}

export class ErrorNotFound extends Error {
  constructor(name: string | URL) {
    super("file not found: " + name.toString());
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

import type * as monacoNS from "monaco-editor-core";

const enc = new TextEncoder();
const dec = new TextDecoder();

export interface VFSInterface {
  readonly ErrorNotFound: typeof ErrorNotFound;
  openModel(name: string | URL): Promise<monacoNS.editor.ITextModel>;
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

interface VFile {
  url: string;
  version: number;
  content: string | Uint8Array;
  ctime: number;
  mtime: number;
  headers?: [string, string][];
}

interface VFSOptions {
  scope?: string;
  initial?: Record<string, string[] | string | Uint8Array>;
}

/** Virtual file system class for monaco editor. */
// TODO: use lz-string to compress text content
export class VFS implements VFSInterface {
  #monaco: typeof monacoNS;
  #db: Promise<IDBDatabase> | IDBDatabase;
  #watchHandlers = new Map<
    string,
    Set<(evt: { kind: string; path: string }) => void>
  >();

  constructor(options: VFSOptions) {
    const req = openDB(
      "vfs:monaco-app/" + (options.scope ?? "main"),
      async (store) => {
        for (const [name, data] of Object.entries(options.initial ?? {})) {
          const url = toUrl(name);
          const now = Date.now();
          const item: VFile = {
            url: url.href,
            version: 1,
            content: Array.isArray(data) && !(data instanceof Uint8Array)
              ? data.join("\n")
              : data,
            ctime: now,
            mtime: now,
          };
          await waitIDBRequest(store.add(item));
        }
      },
    );
    this.#db = req.then((db) => this.#db = db);
  }

  get ErrorNotFound() {
    return ErrorNotFound;
  }

  async #begin(readonly = false) {
    const db = await this.#db;
    return db.transaction("files", readonly ? "readonly" : "readwrite")
      .objectStore("files");
  }

  async openModel(name: string | URL) {
    const monaco = this.#monaco;
    const url = toUrl(name);
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
    const url = toUrl(name);
    const db = await this.#begin(true);
    return waitIDBRequest<string>(db.getKey(url.href)).then((key) => !!key);
  }

  async list() {
    const db = await this.#begin(true);
    const req = db.getAllKeys();
    return await waitIDBRequest<string[]>(req);
  }

  async #read(name: string | URL) {
    const url = toUrl(name);
    const db = await this.#begin(true);
    const ret = await waitIDBRequest<VFile>(db.get(url.href));
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
    const { pathname, href: url } = toUrl(name);
    const db = await this.#begin();
    const old = await waitIDBRequest<VFile>(
      db.get(url),
    );
    const now = Date.now();
    const file: VFile = {
      url,
      version: version ?? (1 + (old?.version ?? 0)),
      content,
      ctime: old?.ctime ?? now,
      mtime: now,
    };
    await waitIDBRequest(db.put(file));
    setTimeout(() => {
      for (const key of [url, "*"]) {
        const handlers = this.#watchHandlers.get(key);
        if (handlers) {
          for (const handler of handlers) {
            handler({ kind: old ? "modify" : "create", path: pathname });
          }
        }
      }
    }, 0);
  }

  async removeFile(name: string | URL): Promise<void> {
    const { pathname, href } = toUrl(name);
    const db = await this.#begin();
    await waitIDBRequest(db.delete(href));
    setTimeout(() => {
      for (const key of [href, "*"]) {
        const handlers = this.#watchHandlers.get(key);
        if (handlers) {
          for (const handler of handlers) {
            handler({ kind: "remove", path: pathname });
          }
        }
      }
    }, 0);
  }

  watch(
    name: string | URL,
    handler: (evt: WatchEvent) => void,
  ): () => void {
    const url = name == "*" ? name : toUrl(name).href;
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

  fetch(url: string | URL) {
    return vfetch(url);
  }

  bindMonaco(monaco: typeof monacoNS) {
    this.#monaco = monaco;
  }
}

/** Error for file not found. */
export class ErrorNotFound extends Error {
  constructor(name: string | URL) {
    super("file not found: " + name.toString());
  }
}

/** Open the given IndexedDB database. */
export function openDB(
  name: string,
  onStoreCreate?: (store: IDBObjectStore) => void | Promise<void>,
) {
  const req = indexedDB.open(name, 1);
  req.onupgradeneeded = () => {
    const db = req.result;
    if (!db.objectStoreNames.contains("files")) {
      const store = db.createObjectStore("files", { keyPath: "url" });
      onStoreCreate?.(store);
    }
  };
  return waitIDBRequest<IDBDatabase>(req);
}

/** wait for the given IDBRequest. */
export function waitIDBRequest<T>(req: IDBRequest): Promise<T> {
  return new Promise((resolve, reject) => {
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

/** The cache storage in IndexedDB. */
let cacheDb: Promise<IDBDatabase> | IDBDatabase | null = null;

/** Fetch with vfs cache. */
export async function vfetch(url: string | URL): Promise<Response> {
  const db = await (cacheDb ?? (cacheDb = openDB("vfs:monaco-cache")));
  const tx = db.transaction("files", "readonly").objectStore("files");
  const caceUrl = toUrl(url).href;
  const ret = await waitIDBRequest<VFile>(tx.get(caceUrl));
  if (ret && ret.headers) {
    const headers = new Headers(ret.headers);
    const cc = headers.get("cache-control");
    let hit = false;
    if (cc) {
      if (cc.includes("immutable")) {
        hit = true;
      } else {
        const m = cc.match(/max-age=(\d+)/);
        if (m) {
          const maxAgeMs = Number(m[1]) * 1000;
          hit = ret.mtime + maxAgeMs > Date.now();
        }
      }
    }
    if (hit) {
      return new Response(ret.content, { headers });
    }
  }
  const res = await fetch(url);
  const cc = res.headers.get("cache-control");
  if (res.ok && cc && (cc.includes("max-age=") || cc.includes("immutable"))) {
    const content = new Uint8Array(await res.arrayBuffer());
    const headers = [...res.headers.entries()].filter(([k]) =>
      ["content-type", "content-length", "cache-control", "x-typescript-types"]
        .includes(k)
    );
    const now = Date.now();
    const file: VFile = {
      url: caceUrl,
      version: 1,
      content,
      headers,
      ctime: now,
      mtime: now,
    };
    const tx = db.transaction("files", "readwrite").objectStore("files");
    await waitIDBRequest<VFile>(tx.put(file));
    return new Response(content, { headers });
  }
  return res;
}

/** Convert string to URL. */
function toUrl(name: string | URL) {
  return typeof name === "string" ? new URL(name, "file:///") : name;
}

/** Convert string to Uint8Array. */
function toUint8Array(data: string | Uint8Array): Uint8Array {
  return typeof data === "string" ? enc.encode(data) : data;
}

/** Convert Uint8Array to string. */
function toString(data: string | Uint8Array) {
  return data instanceof Uint8Array ? dec.decode(data) : data;
}

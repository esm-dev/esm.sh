/*! 🔥 esm.sh/hot
 *  Docs: https://til.esm.sh/hot
 */

/// <reference lib="webworker" />

import type { ArchiveEntry, FireOptions, HotCore, Plugin } from "./server/embed/types/hot.d.ts";

const VERSION = 135;
const doc: Document | undefined = globalThis.document;
const kHot = "esm.sh/hot";
const kMessage = "message";
const kVfs = "vfs";
const kTypeEsmArchive = "application/esm-archive";
const kHotArchive = "#hot-archive";

/** class `VFS` implements the virtual file system by using indexed database. */
class VFS {
  #db: IDBDatabase | Promise<IDBDatabase>;

  constructor(scope: string, version: number) {
    const req = indexedDB.open(scope, version);
    req.onupgradeneeded = () => {
      const db = req.result;
      if (!db.objectStoreNames.contains(kVfs)) {
        db.createObjectStore(kVfs, { keyPath: "name" });
      }
    };
    this.#db = waitIDBRequest<IDBDatabase>(req);
  }

  async #begin(readonly = false) {
    let db = this.#db;
    if (db instanceof Promise) {
      db = this.#db = await db;
    }
    return db.transaction(kVfs, readonly ? "readonly" : "readwrite")
      .objectStore(kVfs);
  }

  async get(name: string) {
    const tx = await this.#begin(true);
    const ret = await waitIDBRequest<File & { content: ArrayBuffer } | undefined>(tx.get(name));
    if (ret) {
      return new File([ret.content], ret.name, ret);
    }
  }

  async put(file: File) {
    const { name, type, lastModified } = file;
    const vfile = { name, type, lastModified, content: await file.arrayBuffer() };
    const tx = await this.#begin();
    return waitIDBRequest<string>(tx.put(vfile));
  }

  async delete(name: string) {
    const tx = await this.#begin();
    return waitIDBRequest<void>(tx.delete(name));
  }
}

/**
 * class `Archive` implements the reader for esm-archive format.
 * more details see https://www.npmjs.com/package/esm-archive
 */
class Archive {
  #buffer: ArrayBuffer;
  #entries: Record<string, ArchiveEntry> = {};

  static invalidFormat = new Error("Invalid esm-archive format");

  constructor(buffer: ArrayBuffer) {
    this.#buffer = buffer;
    this.#parse();
  }

  public checksum: number;

  #parse() {
    const dv = new DataView(this.#buffer);
    const decoder = new TextDecoder();
    const readUint32 = (offset: number) => dv.getUint32(offset);
    const readString = (offset: number, length: number) => decoder.decode(new Uint8Array(this.#buffer, offset, length));
    if (this.#buffer.byteLength < 18 || readString(0, 10) !== "ESMARCHIVE") {
      throw Archive.invalidFormat;
    }
    const length = readUint32(10);
    if (length !== this.#buffer.byteLength) {
      throw Archive.invalidFormat;
    }
    this.checksum = readUint32(15);
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

  has(name: string) {
    return name in this.#entries;
  }

  readFile(name: string) {
    const info = this.#entries[name];
    return info ? new File([this.#buffer.slice(info.offset, info.offset + info.size)], info.name, info) : null;
  }
}

/** class `Hot` implements the `HotCore` interface. */
class Hot implements HotCore {
  #vfs = new VFS(kHot, VERSION);
  #swScript: string | null = null;
  #swActive: ServiceWorker | null = null;
  #archive: Archive | null = null;
  #fireListeners: ((sw: ServiceWorker) => void)[] = [];
  #promises: Promise<any>[] = [];
  #bc = new BroadcastChannel(kHot);

  get vfs() {
    return this.#vfs;
  }

  onUpdateFound = () => location.reload();

  onFire(handler: (reg: ServiceWorker) => void) {
    if (this.#swActive) {
      handler(this.#swActive);
    } else {
      this.#fireListeners.push(handler);
    }
    return this;
  }

  waitUntil(...promises: readonly Promise<void>[]) {
    this.#promises.push(...promises);
    return this;
  }

  use(...plugins: readonly Plugin[]) {
    plugins.forEach((plugin) => plugin.setup(this));
    return this;
  }

  async fire(options: FireOptions = {}) {
    const sw = navigator.serviceWorker;
    if (!sw) {
      throw new Error("Service Worker not supported");
    }

    const { main, swScript = "/sw.js", swUpdateViaCache } = options;

    // add preload link for the main module if it's provided
    if (main) {
      appendElement("link", { rel: "modulepreload", href: main });
    }

    if (this.#swScript === swScript) {
      return;
    }
    this.#swScript = swScript;

    // register Service Worker
    const reg = await sw.register(this.#swScript, {
      type: "module",
      updateViaCache: swUpdateViaCache,
    });
    const tryFireApp = async () => {
      if (reg.active?.state === "activated") {
        await this.#fireApp(reg.active);
        main && appendElement("script", { type: "module", src: main });
      }
    };

    // detect Service Worker update available and wait for it to become installed
    reg.onupdatefound = () => {
      const { installing } = reg;
      if (installing) {
        installing.onstatechange = () => {
          const { waiting } = reg;
          // it's first install
          if (waiting && !sw.controller) {
            waiting.onstatechange = tryFireApp;
          }
        };
      }
    };

    // detect controller change
    sw.oncontrollerchange = this.onUpdateFound;

    // fire app immediately if there's an activated Service Worker
    tryFireApp();
  }

  async #fireApp(swActive: ServiceWorker) {
    // download and send esm archive to Service Worker
    queryElements<HTMLLinkElement>(`link[rel="preload"][as="fetch"][type="${kTypeEsmArchive}"][href]`, (el) => {
      this.#promises.push(
        fetch(el.href).then((res) => {
          if (res.ok) {
            return res.arrayBuffer();
          }
          return Promise.reject(new Error(res.statusText ?? `<${res.status}>`));
        }).then((arrayBuffer) => {
          swActive.postMessage({ HOT_ARCHIVE: arrayBuffer });
          this.#bc.onmessage = (evt) => {
            if (evt.data === kHotArchive) {
              this.onUpdateFound();
            }
          };
        }).catch((err) => {
          console.error("Failed to fetch", el.href, err[kMessage]);
        }),
      );
    });

    // wait until all promises resolved
    await Promise.all(this.#promises);
    this.#promises = [];

    // fire all `fire` listeners
    for (const handler of this.#fireListeners) {
      handler(swActive);
    }
    this.#fireListeners = [];
    this.#swActive = swActive;

    // apply "[type=hot/module]" script tags
    queryElements<HTMLScriptElement>("script[type='hot/module']", (el) => {
      const copy = el.cloneNode(true) as HTMLScriptElement;
      copy.type = "module";
      el.replaceWith(copy);
    });
  }

  listen() {
    // @ts-expect-error missing types
    if (typeof clients === "undefined") {
      throw new Error("Service Worker scope not found.");
    }

    const vfs = this.#vfs;
    const on: typeof addEventListener = addEventListener;
    const serveVFS = async (name: string) => {
      const file = await vfs.get(name);
      if (!file) {
        return createResponse("Not Found", {}, 404);
      }
      const headers: HeadersInit = { "content-type": file.type };
      return createResponse(file, headers);
    };

    this.#promises.push(
      vfs.get(kHotArchive).then(async (file) => {
        if (file) {
          this.#archive = new Archive(await file.arrayBuffer());
        }
      }).catch((err) => console.error(err[kMessage])),
    );

    on("install", (evt) => {
      // @ts-expect-error missing types
      skipWaiting();
      evt.waitUntil(Promise.all(this.#promises));
    });

    on("activate", (evt) => {
      // @ts-expect-error missing types
      evt.waitUntil(clients.claim());
    });

    on("fetch", (evt) => {
      const { request } = evt;
      const respondWith = evt.respondWith.bind(evt);
      const url = new URL(request.url);
      const { pathname } = url;
      const archive = this.#archive;
      if (url.origin === location.origin && pathname.startsWith("/@hot/")) {
        respondWith(serveVFS(pathname.slice(6)));
      }
      for (const key of [request.url, pathname]) {
        if (archive?.has(key)) {
          const file = archive.readFile(key)!;
          respondWith(createResponse(file, { "content-type": file.type }));
          break;
        }
      }
    });

    on(kMessage, (evt) => {
      const { data } = evt;
      if (typeof data === "object" && data !== null) {
        const buffer = data.HOT_ARCHIVE;
        if (buffer instanceof ArrayBuffer) {
          try {
            const archive = new Archive(buffer);
            if (archive.checksum !== this.#archive?.checksum) {
              this.#archive = archive;
              this.#bc.postMessage(kHotArchive);
              vfs.put(new File([buffer], kHotArchive, { type: kTypeEsmArchive }));
            }
          } catch (err) {
            console.error(err[kMessage]);
          }
        }
      }
    });
  }
}

/** query all elements by the given selectors. */
function queryElements<T extends Element>(
  selectors: string,
  callback: (value: T) => void,
) {
  // @ts-expect-error throw error if document is not available
  doc.querySelectorAll(selectors).forEach(callback);
}

/** create a response object. */
function createResponse(
  body: BodyInit | null,
  headers: HeadersInit = {},
  status = 200,
): Response {
  return new Response(body, { headers, status });
}

/** append an element to the document. */
function appendElement(tag: string, attrs: Record<string, string>, pos: "head" | "body" = "head") {
  const el = doc!.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    el[k] = v;
  }
  doc![pos].appendChild(el);
}

/** wait for the given IDBRequest. */
function waitIDBRequest<T>(req: IDBRequest): Promise<T> {
  return new Promise((resolve, reject) => {
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

export const hot = new Hot();
export default hot;

/*! 🔥 esm.sh/hot
 *  Docs: https://docs.esm.sh/hot
 */

/// <reference lib="webworker" />

import type { ArchiveEntry, FireOptions, HotCore, Plugin } from "./types/hot";

const VERSION = 135;
const doc: Document | undefined = globalThis.document;
const kHot = "esm.sh/hot";
const kMessage = "message";
const kVfs = "vfs";
const kTypeEsmArchive = "application/esm-archive";
const kHotArchive = "#hot-archive";

/** class `VFS` implements the virtual file system by using indexed database. */
class VFS {
  private _db: IDBDatabase | Promise<IDBDatabase>;

  constructor(scope: string, version: number) {
    const req = indexedDB.open(scope, version);
    req.onupgradeneeded = () => {
      const db = req.result;
      if (!db.objectStoreNames.contains(kVfs)) {
        db.createObjectStore(kVfs, { keyPath: "name" });
      }
    };
    this._db = waitIDBRequest<IDBDatabase>(req);
  }

  private async _tx(readonly = false) {
    let db = this._db;
    if (db instanceof Promise) {
      db = this._db = await db;
    }
    return db.transaction(kVfs, readonly ? "readonly" : "readwrite").objectStore(kVfs);
  }

  async has(name: string) {
    const tx = await this._tx(true);
    return await waitIDBRequest<string>(tx.getKey(name)) === name;
  }

  async get(name: string) {
    const tx = await this._tx(true);
    const ret = await waitIDBRequest<File & { content: ArrayBuffer } | undefined>(tx.get(name));
    if (ret) {
      return new File([ret.content], ret.name, ret);
    }
  }

  async put(file: File) {
    const { name, type, lastModified } = file;
    if (await this.has(name)) {
      return name;
    }
    const content = await file.arrayBuffer();
    const tx = await this._tx();
    return waitIDBRequest<string>(tx.put({ name, type, lastModified, content }));
  }

  async delete(name: string) {
    const tx = await this._tx();
    return waitIDBRequest<void>(tx.delete(name));
  }
}

/**
 * class `Archive` implements the reader for esm-archive format.
 * more details see https://www.npmjs.com/package/esm-archive
 */
class Archive {
  private _buf: ArrayBuffer;
  private _files: Record<string, ArchiveEntry> = {};

  static invalidFormat = new Error("Invalid esm-archive format");

  constructor(buffer: ArrayBuffer) {
    this._buf = buffer;
    this._parse();
  }

  public checksum: number;

  private _parse() {
    const dv = new DataView(this._buf);
    const decoder = new TextDecoder();
    const readUint32 = (offset: number) => dv.getUint32(offset);
    const readString = (offset: number, length: number) => decoder.decode(new Uint8Array(this._buf, offset, length));
    if (this._buf.byteLength < 18 || readString(0, 10) !== "ESMARCHIVE") {
      throw Archive.invalidFormat;
    }
    const length = readUint32(10);
    if (length !== this._buf.byteLength) {
      throw Archive.invalidFormat;
    }
    this.checksum = readUint32(14);
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
      this._files[name] = { name, type, lastModified, offset, size };
      offset += size;
    }
  }

  exists(filename: string) {
    return filename in this._files;
  }

  openFile(filename: string) {
    const { name, offset, size, ...rest } = this._files[filename];
    return new File([this._buf.slice(offset, offset + size)], name, rest);
  }
}

/** class `Hot` implements the `HotCore` interface. */
class Hot implements HotCore {
  private _vfs: VFS | null = null;
  private _swScript: string | null = null;
  private _swActive: ServiceWorker | null = null;
  private _archive: Archive | null = null;
  private _fetchListeners: ((event: FetchEvent) => void)[] = [];
  private _fireListeners: ((sw: ServiceWorker) => void)[] = [];
  private _promises: Promise<any>[] = [];

  get vfs() {
    return this._vfs ?? (this._vfs = new VFS(kHot, VERSION));
  }

  use(...plugins: readonly Plugin[]) {
    plugins.forEach((plugin) => plugin.setup(this));
    return this;
  }

  onUpdateFound = () => location.reload();

  onFetch(handler: (event: FetchEvent) => void) {
    this._fetchListeners.push(handler);
    return this;
  }

  onFire(handler: (reg: ServiceWorker) => void) {
    if (this._swActive) {
      handler(this._swActive);
    } else {
      this._fireListeners.push(handler);
    }
    return this;
  }

  waitUntil(...promises: readonly Promise<void>[]) {
    this._promises.push(...promises);
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

    if (this._swScript === swScript) {
      return;
    }
    this._swScript = swScript;

    // register Service Worker
    const reg = await sw.register(this._swScript, {
      type: "module",
      updateViaCache: swUpdateViaCache,
    });
    const tryFireApp = async () => {
      if (reg.active?.state === "activated") {
        await this._fireApp(reg.active);
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

  private async _fireApp(swActive: ServiceWorker) {
    // download and send esm archive to Service Worker
    queryElements<HTMLLinkElement>(`link[rel=preload][as=fetch][type^="${kTypeEsmArchive}"][href]`, (el) => {
      this._promises.push(
        fetch(el.href).then((res) => {
          if (res.ok) {
            if (el.type.endsWith("+gzip")) {
              res = new Response(res.body?.pipeThrough(new DecompressionStream("gzip")));
            }
            return res.arrayBuffer();
          }
          return Promise.reject(new Error(res.statusText ?? `<${res.status}>`));
        }).then((arrayBuffer) => {
          swActive.postMessage({ [kHotArchive]: arrayBuffer });
          new BroadcastChannel(kHot).onmessage = (evt) => {
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
    await Promise.all(this._promises);
    this._promises = [];

    // fire all `fire` listeners
    for (const handler of this._fireListeners) {
      handler(swActive);
    }
    this._fireListeners = [];
    this._swActive = swActive;

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

    const bc = new BroadcastChannel(kHot);
    const vfs = this.vfs;
    const on: typeof addEventListener = addEventListener;
    const serveVFS = async (name: string) => {
      const file = await vfs.get(name);
      if (!file) {
        return createResponse("Not Found", {}, 404);
      }
      return createResponse(file, { "content-type": file.type });
    };

    this._promises.push(
      vfs.get(kHotArchive).then(async (file) => {
        if (file) {
          this._archive = new Archive(await file.arrayBuffer());
        }
      }).catch((err) => console.error(err[kMessage])),
    );

    on("install", (evt) => {
      // @ts-expect-error missing types
      skipWaiting();
      evt.waitUntil(Promise.all(this._promises));
    });

    on("activate", (evt) => {
      // @ts-expect-error missing types
      evt.waitUntil(clients.claim());
    });

    on("fetch", (evt) => {
      const { request } = evt as FetchEvent;
      const url = new URL(request.url);
      const { pathname } = url;
      const isSameOrigin = url.origin === location.origin;
      const pathOrHref = isSameOrigin ? pathname : request.url;
      const archive = this._archive;
      const listeners = this._fetchListeners;
      const respondWith = evt.respondWith.bind(evt);
      if (isSameOrigin && pathname.startsWith("/@hot/")) {
        respondWith(serveVFS(pathname.slice(6)));
      } else if (archive?.exists(pathOrHref)) {
        const file = archive.openFile(pathOrHref)!;
        respondWith(createResponse(file, { "content-type": file.type }));
      } else if (listeners.length > 0) {
        let responded = false;
        evt.respondWith = (res: Response | Promise<Response>) => {
          responded = true;
          respondWith(res);
        };
        for (const handler of listeners) {
          if (responded) {
            break;
          }
          handler(evt);
        }
      }
    });

    on(kMessage, (evt) => {
      const { data } = evt;
      if (typeof data === "object" && data !== null) {
        const buffer = data[kHotArchive];
        if (buffer instanceof ArrayBuffer) {
          try {
            const archive = new Archive(buffer);
            if (archive.checksum !== this._archive?.checksum) {
              this._archive = archive;
              bc.postMessage(kHotArchive);
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
function appendElement(tag: string, attrs: Record<string, string>, parent: "head" | "body" = "head") {
  const el = doc!.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    el[k] = v;
  }
  doc![parent].appendChild(el);
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

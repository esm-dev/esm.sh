/*! 🔥 esm.sh/run - speeding up your modern(es2015+) web application with service worker.
 *  Docs: https://docs.esm.sh/run
 */

import type { VFile } from "./types/run.d.ts";

const global = globalThis;
const clients: Clients | undefined = global.clients;
const on = global.addEventListener;
const kRun = "esm.sh/run";
const kVFSDBStoreName = "files";
const bc = new BroadcastChannel(kRun);

let vfsDB: Promise<IDBDatabase> | IDBDatabase = openVFSDB();
let esmBundle: Record<string, string> | undefined;

async function openVFSDB() {
  const req = indexedDB.open(kRun);
  req.onupgradeneeded = () => {
    const db = req.result;
    if (!db.objectStoreNames.contains(kVFSDBStoreName)) {
      db.createObjectStore(kVFSDBStoreName, { keyPath: "url" });
    }
  };
  const db = await promisifyIDBRequest<IDBDatabase>(req);
  db.onclose = () => {
    // reopen the db when it's closed
    vfsDB = openVFSDB();
  };
  return vfsDB = db;
}

async function tx(readonly = false) {
  const db = await vfsDB;
  return db.transaction(kVFSDBStoreName, readonly ? "readonly" : "readwrite").objectStore(kVFSDBStoreName);
}

async function readFileFromVFS(name: string | URL): Promise<Uint8Array | null> {
  const store = await tx(true);
  const ret = await promisifyIDBRequest<VFile | undefined>(store.get(normalizeUrl(name)));
  return ret?.content ?? null;
}

async function writeFileToVFS(
  name: string | URL,
  content: Uint8Array,
  options?: { contentType?: string; lastModified?: number },
): Promise<void> {
  const store = await tx();
  const file: VFile = {
    ...options,
    url: normalizeUrl(name),
    content,
  };
  return promisifyIDBRequest(store.put(file));
}

async function loadEsmBundleFromVFS() {
  const jsonContent = await readFileFromVFS(".esm-bundle.json");
  if (jsonContent) {
    esmBundle = await parseEsmBundle(jsonContent.buffer);
  }
}

async function parseEsmBundle(jsonContent: ArrayBuffer): Promise<Record<string, string>> {
  const v = JSON.parse(new TextDecoder().decode(jsonContent));
  if (typeof v !== "object" || v === null) {
    throw new Error("Invalid esm-bundle: the content is not an object");
  }
  v.$checksum = await shasum(jsonContent);
  return v;
}

on("install", (evt) => {
  // @ts-expect-error The `skipWaiting` method of the `ServiceWorkerGlobalScope` interface
  // forces the waiting service worker to become the active service worker.
  skipWaiting();
  // query the esm-bundle from cache and load it into memory if exists
  evt.waitUntil(loadEsmBundleFromVFS());
});

on("activate", (evt) => {
  // When a service worker is initially registered, pages won't use it until they next load.
  // The `clients.claim()` method causes those pages to be controlled immediately.
  evt.waitUntil(clients!.claim());
});

on("fetch", (evt) => {
  const { request } = evt as FetchEvent;
  const url = new URL(request.url);
  const { pathname } = url;
  const isSameOrigin = url.origin === location.origin;
  const pathOrHref = isSameOrigin ? pathname : request.url;
  if (esmBundle && pathOrHref in esmBundle) {
    evt.respondWith(createResponse(esmBundle[pathOrHref], { "content-type": "application/javascript; charset=utf-8" }));
  }
});

on("message", async (evt) => {
  const { data } = evt;
  if (Array.isArray(data)) {
    const [HEAD, buffer] = data;
    if (HEAD === 0x7f && buffer instanceof ArrayBuffer) {
      try {
        const bundle = await parseEsmBundle(buffer);
        const isStale = !esmBundle || esmBundle.$checksum !== bundle.$checksum;
        if (isStale) {
          esmBundle = bundle;
          writeFileToVFS(".esm-bundle.json", new Uint8Array(buffer));
        }
        bc.postMessage(isStale ? 2 : 1);
      } catch (err) {
        bc.postMessage(0);
        console.error(err);
      }
    }
  }
});

/** create a response object. */
function createResponse(body: BodyInit | null, headers?: HeadersInit, status?: number): Response {
  return new Response(body, { headers, status });
}

/** promisify the given IDBRequest. */
function promisifyIDBRequest<T>(req: IDBRequest): Promise<T> {
  return new Promise((resolve, reject) => {
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

/** normalize the given URL string or URL object. */
function normalizeUrl(url: string | URL) {
  return (typeof url === "string" ? new URL(url, "file:///") : url).href;
}

/** checksum the given input ArrayBuffer with SHA-256 algorithm. */
async function shasum(input: ArrayBuffer): Promise<string> {
  const buf = await crypto.subtle.digest("SHA-256", input);
  return btoa(String.fromCharCode(...new Uint8Array(buf)));
}

function run() {
  throw new Error("calling `run()` in the service worker scope is not allowed");
}

export { run, run as default };

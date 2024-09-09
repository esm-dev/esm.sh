/// <reference lib="webworker" />

const _self: ServiceWorkerGlobalScope = self as unknown as ServiceWorkerGlobalScope;

function setupServiceWorker() {
  // @ts-expect-error `$TARGET` is injected by esbuild
  const target: string = $TARGET;
  const importMap: Record<string, unknown> = {};
  const regexpTsx = /\.(jsx|ts|mts|tsx)$/;
  const cachePromise = caches.open("esm.sh/run");
  const on = _self.addEventListener;

  on("install", (evt) => {
    // The `skipWaiting` method forces the waiting service worker to become
    // the active service worker.
    _self.skipWaiting();
  });

  on("activate", (evt) => {
    // When a service worker is initially registered, pages won't use it until they next load.
    // The `clients.claim()` method causes those pages to be controlled immediately.
    evt.waitUntil(_self.clients.claim());
  });

  on("fetch", (evt) => {
    const { request } = evt as FetchEvent;
    if (request.url.startsWith(location.origin)) {
      const url = new URL(request.url);
      const pathname = url.pathname;
      if (regexpTsx.test(pathname)) {
      }
    }
  });

  on("message", async (evt) => {
    const { data } = evt;
    if (Array.isArray(data)) {
      const [HEAD] = data;
      if (HEAD === "importmap") {
        Object.assign(importMap, data[1]);
      }
    }
  });
}

setupServiceWorker();

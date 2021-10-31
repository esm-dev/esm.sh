import localforage from '/localforage';
import createESMWorker from '/esm-worker';

const worker = createESMWorker({
  readFile: name => localforage.getItem(`file-${name.replace('/embed/playground/', '')}`)
})

self.addEventListener('install', e => {
  e.waitUntil(self.skipWaiting());
});

self.addEventListener('activate', e => {
  e.waitUntil(self.clients.claim());
});

self.addEventListener('fetch', e => {
  const { pathname } = new URL(e.request.url)
  if (pathname.startsWith('/embed/playground/')) {
    e.respondWith(worker.fetch(e.request))
  }
});

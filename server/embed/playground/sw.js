import localforage from '/localforage';
import createESMWorker from '/esm-worker';

let esmWorker 

self.addEventListener('install', e => {
  e.waitUntil(self.skipWaiting());
});

self.addEventListener('activate', e => {
  esmWorker = createESMWorker({
    readFile: name => localforage.getItem(`file-${name.replace('/embed/playground/', '')}`)
  })
  e.waitUntil(self.clients.claim());
});

self.addEventListener('fetch', e => {
  const { pathname } = new URL(e.request.url)
  if (pathname.startsWith('/embed/playground/')) {
    e.respondWith(esmWorker.fetch(e.request))
  }
});

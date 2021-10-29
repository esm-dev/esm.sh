import localforage from '/localforage';

self.addEventListener('install', e => {
  e.waitUntil(self.skipWaiting());
});

self.addEventListener('activate', e => {
  e.waitUntil(self.clients.claim());
});

self.addEventListener('fetch', e => {
  e.respondWith(serve(e.request))
});

async function serve(request) {
  const url = new URL(request.url)
  const file = url.pathname.replace('/embed/playground/', '')
  const content = await localforage.getItem(`file-${file}`)
  if (content) {
    if (file.endsWith('.html')) {
      return new Response(content, {
        headers: {
          'content-type': 'text/html',
        },
      })
    } else if (/\.(js|jsx|ts|tsx)$/.test(file)) {

    } else if (/\.(css)$/.test(file)) {

    }
  }
  return new Response(`not found`, { status: 404 })
}

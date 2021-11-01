import localforage from '/localforage'
import createESMWorker from '/esm-worker'

const esmWorker = createESMWorker({
  fs: {
    readFile: name => localforage.getItem(`file-${name.replace('/embed/playground/', '')}`)
  },
  isDev: true,
  getCompilerWasm: () => fetch('https://esm.sh/esm-worker/compiler/pkg/esm_worker_compiler_bg.wasm', { mode: 'cors' })
})

self.addEventListener('install', e => {
  console.log('sw->install')
  if (location.hostname === 'localhost') {
    e.waitUntil(self.skipWaiting())
  }
})

self.addEventListener('activate', e => {
  console.log('sw->activate')
})

self.addEventListener('fetch', e => {
  console.log('sw->fetch', e.request.url)
  const { pathname } = new URL(e.request.url)
  if (pathname !== '/embed/playground/sw.js') {
    e.respondWith(esmWorker.fetch(e.request))
  }
})

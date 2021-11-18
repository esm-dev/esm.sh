import localforage from '/localforage'
import createESMWorker from '/esm-worker'
import initCompiler, { transformSync } from '/esm-compiler'
import initLolHtml, { HTMLRewriter } from '/lol-html-wasm'
import compilerWasm from '/esm-compiler/esm_compiler_bg.wasm'
import lolHtmlWasm from '/lol-html-wasm/lol_html_wasm_bg.wasm'

initCompiler(compilerWasm)
initLolHtml(lolHtmlWasm)

self.HTMLRewriter = HTMLRewriter
self.appFileSystem = {
  readFile: name => localforage.getItem(`file-${name.replace('/embed/playground/', '')}`)
}

const esmWorker = createESMWorker({
  appWorker: {
    appFileSystem = {
      readFile: name => localforage.getItem(`file-${name.replace('/embed/playground/', '')}`)
    },
    fetch: req => {
      return new Response('app: ' + req.url)
    }
  },
  compileWorker: {
    fetch: async (name, sourceCode) => {
      let importMap = {}
      try {
        for (const name of ['import-map.json', 'import_map.json', 'importmap.json']) {
          const data = await appFileSystem.readFile(name)
          const v = JSON.parse(typeof data === 'string' ? data : decoder.decode(data))
          if (v.imports) {
            importMap = v
            break
          }
        }
      } catch (e) { }
      const transformOptions = { importMap, isDev: true }
      const { code } = transformSync(name, sourceCode, transformOptions)
      return new Response(code, {
        headers: {
          'content-type': 'application/javascript',
        },
      })
    }
  }
})

self.addEventListener('install', e => {
  console.log('sw::install')
  if (location.hostname === 'localhost') {
    e.waitUntil(self.skipWaiting())
  }
})

self.addEventListener('activate', e => {
  console.log('sw::activate')
})

let fetchCounter = 0

self.addEventListener('fetch', e => {
  console.log(`sw::fetch [${++fetchCounter}]`, e.request.url)
  const { pathname } = new URL(e.request.url)
  if (pathname.startsWith('/embed/playground/') && pathname !== '/embed/playground/sw.js') {
    e.respondWith(esmWorker.fetch(e.request))
  }
})

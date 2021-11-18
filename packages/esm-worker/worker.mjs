import { getContentType } from './mime.mjs'

const decoder = new TextDecoder()
const isObject = v => typeof v === 'object' && v !== null && !Array.isArray(v)
const concatArrayBuffers = (...bufs) => {
	const array = new Uint8Array(bufs.reduce((totalSize, buf) => totalSize + buf.byteLength, 0));
	bufs.reduce((offset, buf) => {
		array.set(buf, offset)
		return offset + buf.byteLength
	}, 0)
	return array.buffer
}

export default function createESMWorker(options) {
	const { appFileSystem, appStorage, compileWorker, loadWorker, ssrWorker, isDev } = options

	return {
		async fetch(request) {
			const { pathname, searchParams } = new URL(request.url)
			const importMap = { imports: {}, scope: {}, jsx: {} }
			const indexHtmlFile = '/index.html' // todo: support MPA

			let indexHtml = await appFileSystem.readFile(indexHtmlFile)
			let importMapFile = null
			let ssr = false

			// check and load importMap
			if (indexHtml) {
				const chunks = []
				const wr = new HTMLRewriter('utf-8', chunk => chunks.push(chunk))
				wr.on('script[type="importmap"]', {
					element(el) {
						importMapFile = el.getAttribute('src')
						if (src) {
							el.removeAttribute('src')
							el.setInnerContent('$IMPORTMAP')
						}
					},
					text(text) {
						try {
							const v = JSON.parse(text)
							if (isObject(v)) {
								Object.assign(importMap, v)
							}
						} catch (e) { }
					}
				})
				wr.on('script[type="ssr"]', {
					element() {
						ssr = true
					}
				})
				wr.write(indexHtml)
				wr.end()
				indexHtml = concatArrayBuffers(chunks)
			}

			if (importMapFile) {
				const url = new URL(importMapFile, `http://ws${indexHtmlFile}`)
				const data = await appFileSystem.readFile(url.pathname)
				try {
					const v = JSON.parse(decoder.decode(data))
					if (isObject(v)) {
						Object.assign(importMap, v)
					}
				} catch (e) { }
			}

			// serve static
			if (!pathname.endsWith('.html')) {
				let content = await appFileSystem.readFile(pathname)
				if (content) {
					// apply loaders 
					for (const [pattern, src] of Object.entries(importMap.imports)) {
						if (src.endsWith('!loader')) {
							try {
								const reg = new RegExp(pattern)
								if (reg.test(pathname)) {
									const resp = await loadWorker.fetch(new Request("", {
										headers: new Headers({ pattern }),
										body: content,
									}))
									content = resp.arrayBuffer()
									break
								}
							} catch (e) { }
						}
					}

					// compile source code
					if (compileWorker && typeof compileWorker.fetch === 'function') {
						if (
							/\.(js|jsx|mjs|ts|tsx|mts|vue|svelte|mdx)$/.test(pathname) ||
							(/\.(md|css)$/.test(pathname) && searchParams.has('module'))
						) {
							return compileWorker.fetch(new Request("", {
								body: JSON.stringify({
									name: pathname,
									code: decoder.decode(content),
									options: {
										importMap,
										isDev
									}
								}),
								headers: new Headers({ 'content-type': 'application/json' })
							}))
						}
					}

					// static files
					return new Response(content, {
						headers: {
							'content-type': getContentType(pathname),
						},
					})
				}
			}

			// ssr
			if (ssr && ssrWorker && typeof ssrWorker.fetch === 'function') {
				return await ssrWorker.fetch(request)
			}

			// fallback to the index.html
			if (indexHtml) {
				return new Response(indexHtml, {
					headers: { 'content-type': 'text/html' }
				})
			}

			// 404 - not found
			const e404Html = await appFileSystem.readFile('/404.html')
			return new Response(e404Html || '<p><b>404</b> - not found</p>', {
				status: 404,
				headers: { 'content-type': 'text/html' }
			})
		}
	}
}

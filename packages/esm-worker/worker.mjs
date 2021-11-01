import init, { transformSync } from './compiler/pkg/esm_worker_compiler.mjs'
import { getContentType } from './mime.mjs'

export default function createESMWorker(options) {
	const { fs, getCompilerWasm, isDev } = options
	const decoder = new TextDecoder()

	let wasmReady = false

	return {
		async fetch(request) {
			const { pathname } = new URL(request.url)
			const content = await fs.readFile(pathname)
			if (content) {
				if (/\.(js|jsx|ts|tsx)$/.test(pathname)) {
					let importMap = {}
					try {
						for (const name of ['import-map.json', 'import_map.json', 'importmap.json']) {
							const data = await fs.readFile(name)
							const v = JSON.parse(typeof data === 'string' ? data : decoder.decode(data))
							if (v.imports) {
								importMap = v
								break
							}
						}
					} catch (e) { }
					if (wasmReady === false) {
						wasmReady = init(getCompilerWasm()).then(() => wasmReady = true)
					}
					if (wasmReady instanceof Promise) {
						await wasmReady
					}
					const transformOptions = { importMap, isDev }
					const rawCode = typeof content === 'string' ? content : decoder.decode(content)
					const { code } = transformSync(pathname, rawCode, transformOptions)
					return new Response(code, {
						headers: {
							'content-type': 'application/javascript',
						},
					})
				} else {
					return new Response(content, {
						headers: {
							'content-type': getContentType(pathname),
						},
					})
				}
			}
			return new Response(`not found`, { status: 404 })
		}
	}
}

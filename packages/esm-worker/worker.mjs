import init, { transformSync } from './compiler/pkg/esm_worker_compiler.mjs'
import WASM from './compiler/pkg/esm_worker_compiler_bg.wasm'
import { getContentType } from './mime.mjs'

init(WASM)

export default function createESMWorker(fs) {
	const decoder = new TextDecoder()

	return {
		async fetch(request) {
			const { pathname } = new URL(request.url)
			const content = await fs.readFile(pathname)
			if (content) {
				if (/\.(js|jsx|ts|tsx)$/.test(pathname)) {
					let importMap = {}
					try {
						const data = await fs.readFile('import-map.json')
						const v = JSON.parse(typeof data === 'string' ? data : decoder.decode(data))
						if (v.imports) {
							importMap = v
						}
					} catch (e) { }
					const options = { importMap }
					const { code } = transformSync(pathname, typeof content === 'string' ? content : decoder.decode(content), options)
					return new Response(code, {
						headers: {
							'content-type': 'application/javascript',
						},
					})
				} else if (/\.(css)$/.test(pathname)) {
					return new Response(content, {
						headers: {
							'content-type': 'text/css',
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

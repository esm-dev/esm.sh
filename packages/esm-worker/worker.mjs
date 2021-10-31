import init, { transformSync } from './compiler/pkg/esm_worker_compiler.mjs'
import WASM from './compiler/pkg/esm_worker_compiler_bg.wasm'
import { getContentType } from './mime.mjs'

init(WASM)

export default function createESMWorker(fs) {
	return {
		async fetch(request) {
			const { pathname } = new URL(request.url) 
			const content = fs.readFile(pathname)
			if (content) {
				if (/\.(js|jsx|ts|tsx)$/.test(pathname)) {
					const { code } = transformSync(pathname, content)
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
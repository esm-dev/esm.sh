import React from 'http://localhost:8080/react'
import { renderToString } from 'http://localhost:8080/react-dom/server'

Deno.test('check react server rendering', async () => {
	const html = renderToString(<h1>Hi :)</h1>)
	console.log(html)
})

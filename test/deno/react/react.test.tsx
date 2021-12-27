import React from 'http://localhost:8080/react@17'
import { renderToString } from 'http://localhost:8080/react-dom@17/server'
import { assert } from 'https://deno.land/std@0.119.0/testing/asserts.ts'

Deno.test('check react server rendering', async () => {
	const html = renderToString(<main><h1>Hi :)</h1></main>)
	assert(
		typeof html === 'string' &&
		html.includes('<h1>Hi :)</h1>') &&
		html.includes('<main') && html.includes('</main>')
	)
})

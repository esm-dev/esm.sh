import { h } from 'http://localhost:8080/preact'
import { useState } from 'http://localhost:8080/preact/hooks'
import render from 'http://localhost:8080/preact-render-to-string'
import { assert } from 'https://deno.land/std@0.125.0/testing/asserts.ts'

Deno.test('check react server rendering', async () => {
	const App = () => {
		const [message] = useState('Hi :)')
		return <main><h1>{message}</h1></main>
	}
	const html = render(<App />)
	assert(html == '<main><h1>Hi :)</h1></main>')
})

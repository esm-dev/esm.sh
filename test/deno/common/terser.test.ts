import { minify } from 'http://localhost:8080/terser'
import { assert } from 'https://deno.land/std@0.106.0/testing/asserts.ts'

Deno.test('check offical typescript', async () => {
	var code = "function add(first, second) { return first + second; }";
	var result = await minify(code, { sourceMap: true });
	assert(result.code === 'function add(n,d){return n+d}')
	assert(JSON.parse(String(result.map)).names?.length === 3)
})

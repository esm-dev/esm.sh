import * as ts from 'http://localhost:8080/typescript'
import { assert } from 'https://deno.land/std@0.106.0/testing/asserts.ts'

Deno.test('check offical typescript', async () => {
	assert(typeof ts.version === 'string')
	assert(typeof ts.transpileModule === 'function')
})

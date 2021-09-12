import { generate } from 'http://localhost:8080/astring'
import { getURLParams } from 'http://localhost:8080/@tinyhttp/url'
import { assert } from 'https://deno.land/std@0.106.0/testing/asserts.ts'

Deno.test('check marked wth safeLoadFront parser', async () => {
	assert(typeof generate === 'function')
	assert(typeof getURLParams === 'function')
})

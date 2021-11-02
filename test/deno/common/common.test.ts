import { generate } from 'http://localhost:8080/astring'
import { getURLParams } from 'http://localhost:8080/@tinyhttp/url'
import { Sha256 } from 'http://localhost:8080/@aws-crypto/sha256-browser'
import { assert } from 'https://deno.land/std@0.106.0/testing/asserts.ts'

Deno.test('check common modules', async () => {
	assert(typeof generate === 'function')
	assert(typeof getURLParams === 'function')
	assert(typeof Sha256 === 'function')
})

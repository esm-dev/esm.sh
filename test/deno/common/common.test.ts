import { generate } from 'http://localhost:8080/astring'
import { getURLParams } from 'http://localhost:8080/@tinyhttp/url'
import { Sha256 } from 'http://localhost:8080/@aws-crypto/sha256-browser'
import { assertEquals } from 'https://deno.land/std@0.128.0/testing/asserts.ts'

Deno.test('check common modules', async () => {
	assertEquals(typeof generate, 'function')
	assertEquals(typeof getURLParams, 'function')
	assertEquals(typeof Sha256, 'function')
})

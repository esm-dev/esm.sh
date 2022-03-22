import { assertEquals } from 'https://deno.land/std@0.130.0/testing/asserts.ts'

import { generate } from 'http://localhost:8080/astring'
import { getURLParams } from 'http://localhost:8080/@tinyhttp/url'
import { Sha256 } from 'http://localhost:8080/@aws-crypto/sha256-browser'
import compareVersions from "https://esm.sh/tiny-version-compare@3.0.1";

Deno.test('check common modules', async () => {
	assertEquals(typeof generate, 'function')
	assertEquals(typeof getURLParams, 'function')
	assertEquals(typeof Sha256, 'function')
	assertEquals(compareVersions("1.12.0", "v1.12.0"), 0)
})

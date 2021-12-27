import Web3 from 'http://localhost:8080/web3'
import { assert } from 'https://deno.land/std@0.119.0/testing/asserts.ts'

Deno.test('check modules are using nodejs builtin modules', async () => {
	assert(typeof Web3 === 'function')
})

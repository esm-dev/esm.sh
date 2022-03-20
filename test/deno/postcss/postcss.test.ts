import { assert } from 'https://deno.land/std@0.130.0/testing/asserts.ts'

import postcss from 'http://localhost:8080/postcss'
import autoprefixer from 'http://localhost:8080/autoprefixer'

Deno.test('check postcss wth autoprefixer plugin', async () => {
	const { css } = await postcss([autoprefixer]).process(`
		backdrop-filter: blur(5px);
		user-select: none;
	`).async()
	assert(
		typeof css === 'string' &&
		css.includes('-webkit-backdrop-filter: blur(5px);') &&
		css.includes('-webkit-user-select: none;')
	)
})

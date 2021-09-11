import postcss from 'http://localhost:8080/postcss'
import autoprefixer from 'http://localhost:8080/autoprefixer'

Deno.test("check postcss wth autoprefixer plugin", async () => {
	const { css } = await postcss([autoprefixer]).process(`
  backdrop-filter: blur(5px);
  user-select: none;
`).async()

	console.log(css)
})

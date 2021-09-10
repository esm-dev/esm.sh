import postcss from 'http://localhost/postcss'
import autoprefixer from 'http://localhost/autoprefixer'

const { css } = await postcss([autoprefixer]).process(`
  backdrop-filter: blur(5px);
  user-select: none;
`).async()

console.log(css)
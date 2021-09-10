import url from 'http://localhost/@material-ui/core@5.0.0-beta.2/Popper'
import React from 'http://localhost/react'
import { renderToString } from 'http://localhost/react-dom/server'

const html = renderToString(<h1>Hi :)</h1>)
console.log(html)
console.log(url)
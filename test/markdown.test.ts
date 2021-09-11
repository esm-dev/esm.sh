import marked from 'http://localhost:8080/marked@2.0.1'
import { safeLoadFront } from 'http://localhost:8080/yaml-front-matter@4.1.1'

const md = `---
title: esm.sh
---

# ems.sh

A fast, global content delivery network to transform [NPM](http://npmjs.org/) packages to standard **ES Modules** by [esbuild](https://github.com/evanw/esbuild).
`

Deno.test("check marked wth safeLoadFront parser", async () => {
	const { __content, ...meta } = safeLoadFront(md)
	const html = marked.parse(__content)
	console.log(meta, html)
})

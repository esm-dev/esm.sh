import marked from 'http://localhost/marked@2.0.1'
import { safeLoadFront } from 'http://localhost/yaml-front-matter@4.1.1'

const md = `---
title: esm.sh
---

# ems.sh

A fast, global content delivery network to transform [NPM](http://npmjs.org/) packages to standard **ES Modules** by [esbuild](https://github.com/evanw/esbuild).
`

const { __content, ...meta } = safeLoadFront(md)
const html = marked.parse(__content)
console.log(meta, html)
![esm.sh](./server/embed/assets/og-image.svg)

# esm.sh

A fast, global content delivery network for [NPM](http://npmjs.org/) packages with **ES Module** format.

## Import from URL

```javascript
import React from "https://esm.sh/react@18.2.0"
```

You may also use a [semver](https://docs.npmjs.com/cli/v6/using-npm/semver) or a [dist-tag](https://docs.npmjs.com/cli/v8/commands/npm-dist-tag) instead of a fixed version number, or omit the version/tag entirely to use the `latest` tag:

```javascript
import React from "https://esm.sh/react"      // 18.2.0 (latest)
import React from "https://esm.sh/react@17"   // 17.0.2
import React from "https://esm.sh/react@next" // 18.3.0-next-3de926449-20220927
```

### Submodule

```javascript
import { renderToString } from "https://esm.sh/react-dom@18.2.0/server"
```

or import non-module(js) files:

```javascript
import "https://esm.sh/react@18.2.0/package.json" assert { type: "json" }
```

### Specify Dependencies

By default, esm.sh rewrites import specifiers based on the `dependencies` field of `package.json`. To specify version of these dependencies, you can add the `?deps=PACKAGE@VERSION` query. Separate multiple dependencies with comma: `?deps=react@17.0.2,react-dom@17.0.2`.

```javascript
import React from "https://esm.sh/react@17.0.2"
import useSWR from "https://esm.sh/swr?deps=react@17.0.2"
```

### Specify External Dependencies

You can add the `?external=foo,bar` query to specify external dependencies.
Since these dependencies are not resolved, you need to use [**import maps**](https://github.com/WICG/import-maps) to specify the URL for these dependencies. If you are using [Deno](https://deno.land/), you can use the [CLI Script](#using-cli-script) to generate and update the import maps that will resolve the external dependencies automatically.

```json
{
  "imports": {
    "preact": "https://esm.sh/preact@10.7.2",
    "preact-render-to-string": "https://esm.sh/preact-render-to-string@5.2.0?external=preact",
  }
}
```

Or you can **mark all dependencies as external** by adding `*` prefix before the package name:

```json
{
  "imports": {
    "preact": "https://esm.sh/preact@10.7.2",
    "preact-render-to-string": "https://esm.sh/*preact-render-to-string@5.2.0",
    "swr": "https://esm.sh/*swr@1.3.0",
    "react": "https://esm.sh/preact@10.7.2/compat",
  }
}
```

Import maps supports [**trailing slash**](https://github.com/WICG/import-maps#packages-via-trailing-slashes) that can not work with URL search params friendly. To fix this issue, esm.sh provides a **special format** for import URL that allows you to use query params with trailing slash: change the query prefix `?` to `&` and put it after the package version.

```json
{
  "imports": {
    "react-dom": "https://esm.sh/react-dom@18.2.0?pin=v106&dev",
    "react-dom/": "https://esm.sh/react-dom@18.2.0&pin=v106&dev/",
  }
}
```

### Aliasing Dependencies

```javascript
import useSWR from "https://esm.sh/swr?alias=react:preact/compat"
```

in combination with `?deps`:

```javascript
import useSWR from "https://esm.sh/swr?alias=react:preact/compat&deps=preact@10.5.14"
```

The origin idea was coming from [@lucacasonato](https://github.com/lucacasonato).

### Tree Shaking

By default esm.sh bundles module with all export members, you can specify the `exports` by adding `?exports=foo,bar` query. With esbuild tree shaking, you can get a smaller bundle size:

```js
import { __await, __rest } from "https://esm.sh/tslib" // 7.3KB
import { __await, __rest } from "https://esm.sh/tslib?exports=__await,__rest" // 489B
```

This only works with **ESM** modules, **CJS** modules are not supported.

### Bundle Mode

```javascript
import { Button } from "https://esm.sh/antd?bundle"
```

In **bundle** mode, all dependencies are bundled into a single JS file except the peer dependencies.

### Development Mode

```javascript
import React from "https://esm.sh/react?dev"
```

With the `?dev` option, esm.sh builds modules with `process.env.NODE_ENV` set to `"development"` or based on the condition `development` in the `exports` field of `package.json`. This is useful for libraries that have different behavior in development and production. For example, [React](https://reactjs.org/) will use a different warning message in development mode.

### ESBuild Options

By default, esm.sh checks the `User-Agent` header to determine the build target. You can also specify the `target` by adding `?target`, available targets are: **es2015** - **es2022**, **esnext**, **node**, and **deno**.

```javascript
import React from "https://esm.sh/react?target=es2020"
```

Other supported options of [esbuild](https://esbuild.github.io/):

- [Keep names](https://esbuild.github.io/api/#keep-names)
  ```javascript
  import React from "https://esm.sh/react?keep-names"
  ```
- [Ignore annotations](https://esbuild.github.io/api/#ignore-annotations)
  ```javascript
  import React from "https://esm.sh/react?ignore-annotations"
  ```
- [Sourcemap](https://esbuild.github.io/api/#sourcemap)
  ```javascript
  import React from "https://esm.sh/react?sourcemap"
  ```
  This only supports the `inline` mode.

### Web Worker

esm.sh supports `?worker` query to load the module as a web worker:

```javascript
import workerFactory from "https://esm.sh/monaco-editor/esm/vs/editor/editor.worker?worker"

const worker = workerFactory()
```

You can pass some custom code snippet to the worker when calling the factory function:

```javascript
const workerAddon = `
self.onmessage = function (e) {
  console.log(e.data)
}
`
const worker = workerFactory(workerAddon)
```

### Package CSS

```html
<link rel="stylesheet" href="https://esm.sh/monaco-editor?css">
```

This only works when the package **imports CSS files in JS** directly.

### Specify CJS Exports

If you get an error like `...not provide an export named...`, that means esm.sh can not resolve CJS exports of the module correctly. You can add `?cjs-exports=foo,bar` query to specify the export names:

```javascript
import { NinetyRing, NinetyRingWithBg } from "https://esm.sh/react-svg-spinners@0.3.1?cjs-exports=NinetyRing,NinetyRingWithBg"
```

## Deno Compatibility

**esm.sh** resolves the node internal modules (**fs**, **child_process**, etc.) with [`deno.land/std/node`](https://deno.land/std/node) to support Deno.

```javascript
import postcss from "https://esm.sh/postcss"
import autoprefixer from "https://esm.sh/autoprefixer"

const { css } = await postcss([ autoprefixer ]).process(`
  backdrop-filter: blur(5px);
  user-select: none;
.async()
```

By default esm.sh uses a fixed version of `deno.land/std/node`. You can add the `?deno-std=$VER` query to specify a different version:

```javascript
import postcss from "https://esm.sh/postcss?deno-std=0.128.0"
```

### X-Typescript-Types Header 

You may find the `X-TypeScript-Types` header of responses from ems.sh, if the module has a `types` field in `package.json`. This will allow Deno to automatically download the type definitions for types checking and auto-completion ([link](https://deno.land/manual/typescript/types#using-x-typescript-types-header)).

![Figure #1](./server/embed/assets/sceenshot-deno-types.png)

You can add the `?no-dts` query to disable the `X-TypeScript-Types` header if it's incorrect:

```javascript
import unescape from "https://esm.sh/lodash/unescape?no-dts"
```

### Using CLI Script

**esm.sh** provides a CLI script to manage the imports with **import maps** in [Deno](https://deno.land), it resolves dependencies automatically and always use a pinned build version. To use the CLI script, you need to run the `init` command in your project root directory:

```bash
deno run -A -r https://esm.sh init
```

After initializing, you can use the `deno task esm:[add/update/remove]` commands to manage imports of NPM in the import maps.

```bash
deno task esm:add react react-dom # add packages
deno task esm:add react@17 # add packages with specified version
deno task esm:add react:preact/compat # add packages with alias
deno task esm:update react react-dom # upgrade packages
deno task esm:update # update all packages
deno task esm:remove react react-dom # remove packages
```

## Pinning Build Version

Since we update esm.sh server frequently, the server will rebuild all modules when a patch pushed, sometimes we may break packages that work well before by mistake. To avoid this, you can pin the build version of a module with the `?pin` query, this returns an **immutable** cached module.

```javascript
import React from "https://esm.sh/react@17.0.2?pin=v106"
```

## Global CDN

<img width="150" align="right" src="./server/embed/assets/cf.svg">

The Global CDN of esm.sh is provided by [Cloudflare](https://cloudflare.com), one of the world's largest and fastest cloud network platforms.

## Self-Hosting

To host esm.sh by yourself, check the [hosting](./HOSTING.md) documentation.

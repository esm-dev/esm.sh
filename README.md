# ESM

A fast, global content delivery network for [NPM](http://npmjs.org/) packages with **ES Module** format.

## Import from URL

```javascript
import React from "https://esm.sh/react" // 18.2.0
```

### Specify version

```javascript
import React from "https://esm.sh/react@17.0.2"
```

You may also use a [semver](https://docs.npmjs.com/cli/v6/using-npm/semver) or a [dist-tag](https://docs.npmjs.com/cli/v8/commands/npm-dist-tag) instead of a fixed version number, or omit the version/tag entirely to use the `latest` tag:

```javascript
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

### Specify dependencies

```javascript
import React from "https://esm.sh/react@17.0.2"
import useSWR from "https://esm.sh/swr?deps=react@17.0.2"
```

By default, esm.sh will rewrite import specifier based on the package's dependency statement. To specify version of dependencies, you can use the `?deps=PACKAGE@VERSION` query. You can separate multiple dependencies with commas: `?deps=react@17.0.2,react-dom@17.0.2`.

### Specify external dependencies

```json
{
  "imports": {
    "preact": "https://esm.sh/preact@10.7.2",
    "preact-render-to-string": "https://esm.sh/preact-render-to-string@5.2.0?external=preact",
  }
}
```

You can use the `?external=PACKAGE` query to specify external dependencies. Or you can **mark all dependencies as external** by adding `*` prefix before the package name:

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

These dependencies will not be resolved within the code. You need to use [**import maps**](https://github.com/WICG/import-maps) to specify the url for these dependencies. If you are using [Deno](https://deno.land/), you can use the [CLI Script](#use-cli-script) to generate and update the import map that will resolve the dependencies automatically.

### Aliasing dependencies

```javascript
import useSWR from "https://esm.sh/swr?alias=react:preact/compat"
```

in combination with `?deps`:

```javascript
import useSWR from "https://esm.sh/swr?alias=react:preact/compat&deps=preact@10.5.14"
```

The origin idea was coming from [@lucacasonato](https://github.com/lucacasonato).

### Tree Shaking

By default esm.sh will export all members of the module, you can specify the `exports` by adding `?exports=foo,bar` query:

```js
import { __await, __rest } from "https://esm.sh/tslib" // 7.3KB
import { __await, __rest } from "https://esm.sh/tslib?exports=__await,__rest" // 489B
```

### Bundle mode

```javascript
import { Button } from "https://esm.sh/antd?bundle"
```

In **bundle** mode, all dependencies will be bundled into a single JS file.

### Development mode

```javascript
import React from "https://esm.sh/react?dev"
```

The `?dev` query builds modules with `process.env.NODE_ENV` equals to `development`, that is useful to build modules like **React** to allow you to get more development warn/error details.

### ESBuild options

By default, esm.sh will check the `User-Agent` header to get the build target automatically. You can specify it with the `?target` query. Available targets: **es2015** - **es2022**, **esnext**, **node**, and **deno**.

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

### Package CSS

```javascript
import Daygrid from "https://esm.sh/@fullcalendar/daygrid"
```

```html
<link rel="stylesheet" href="https://esm.sh/@fullcalendar/daygrid?css">
```

This only works when the NPM module imports CSS files in JS directly.

<!--
## Web Worker

esm.sh supports `?worker` mode to load modules as web worker:

```javascript
import editorWorker from "https://esm.sh/monaco-editor/esm/vs/editor/editor.worker?worker"

const worker = editorWorker()
```
-->

## Deno compatibility

**esm.sh** will resolve the node internal modules (**fs**, **child_process**, etc.) with [`deno.land/std/node`](https://deno.land/std/node) to support Deno.

```javascript
import postcss from "https://esm.sh/postcss"
import autoprefixer from "https://esm.sh/autoprefixer"

const { css } = await postcss([ autoprefixer ]).process(`
  backdrop-filter: blur(5px);
  user-select: none;
`).async()
```

By default esm.sh will use a fixed version of `deno.land/std/node`. You can use the `?deno-std=$VER` query to specify a different version:

```javascript
import postcss from "https://esm.sh/postcss?deno-std=0.128.0"
```

### Use CLI Script

The CLI script is using to manage the imports with **import maps**, it will resolve the dependencies automatically and always pin the build version. To use the CLI mode, you need to run the `init` command in your project root directory:

```bash
deno run -A -r https://esm.sh init
```

After initializing, you can use the `deno task npm:[add/update/remove]` commands to manage the npm modules in the import maps.

```bash
deno task npm:add react react-dom # add packages
deno task npm:add react@17 # add packages with specified version
deno task npm:add react:preact/compat # add packages with alias
deno task npm:update react react-dom # upgrade packages
deno task npm:update # update all packages
deno task npm:remove react react-dom # remove packages
```

### X-Typescript-Types

By default, **esm.sh** will respond with a custom `X-TypeScript-Types` HTTP header when the types (`.d.ts`) is defined. This is useful for deno type checks ([link](https://deno.land/manual/typescript/types#using-x-typescript-types-header)).

![Figure #1](./server/embed/assets/sceenshot-deno-types.png)

You can pass the `?no-dts` query to disable the `X-TypeScript-Types` header if some types are incorrect:

```javascript
import unescape from "https://esm.sh/lodash/unescape?no-dts"
```

## Pin the build version

Since we update esm.sh server frequently, sometime we may break packages that work fine previously by mistake, the server will rebuild all modules when the patch pushed. To avoid this, you can **pin** the build version by the `?pin=BUILD_VERSON` query. This will give you an **immutable** cached module.

```javascript
import React from "https://esm.sh/react@17.0.2?pin=v99"
```

## Global CDN

<img width="150" align="right" src="./server/embed/assets/cf.svg">

The Global CDN of esm.sh is provided by [Cloudflare](https://cloudflare.com), one of the world's largest and fastest cloud network platforms.

## Self-Hosting

To host esm.sh by yourself, check the [hosting](./HOSTING.md) documentation.

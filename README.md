![esm.sh](./server/embed/assets/og-image.svg)

# esm.sh

A fast, smart, & global content delivery network (CDN) for modern(es2015+) web development.

[![Release](https://img.shields.io/github/v/release/esm-dev/esm.sh?label=&display_name=tag&sort=semver&style=flat&colorA=232323&colorB=232323&logo=hackthebox&logoColor=eeeeee)](https://github.com/esm-dev/esm.sh/releases)
[![Discord](https://img.shields.io/discord/1097820016893763684?style=flat&colorA=232323&colorB=232323&label=&logo=discord&logoColor=eeeeee)](https://discord.gg/XDbjMeb7pb)
[![Twitter](https://img.shields.io/twitter/follow/jexia_?label=%40jexia_&style=flat&colorA=232323&colorB=232323&logo=twitter&logoColor=eeeeee)](https://twitter.com/jexia_)
[![Github Sponsors](https://img.shields.io/github/sponsors/ije?label=&style=flat&colorA=232323&colorB=232323&logo=githubsponsors&logoColor=eeeeee)](https://github.com/sponsors/ije)
[![Open Collective](https://img.shields.io/opencollective/all/esm?label=&style=flat&colorA=232323&colorB=232323&logo=opencollective&logoColor=eeeeee)](https://opencollective.com/esm)

## Import from NPM

```javascript
import React from "https://esm.sh/react@18.2.0"
```

You may also use a [semver](https://docs.npmjs.com/cli/v6/using-npm/semver) or a [dist-tag](https://docs.npmjs.com/cli/v8/commands/npm-dist-tag) instead of a fixed version number, or omit the version/tag entirely to use the `latest` tag:

```javascript
import React from "https://esm.sh/react"      // 18.2.0 (latest)
import React from "https://esm.sh/react@17"   // 17.0.2
import React from "https://esm.sh/react@canary" // 18.3.0-canary-7cd98ef2b-20230509
```

### Submodule

```javascript
import { renderToString } from "https://esm.sh/react-dom@18.2.0/server"
```

or import non-module(js) as following:

```javascript
import "https://esm.sh/react@18.2.0/package.json" assert { type: "json" }
```

### Import from GitHub Repo

You can also import modules/assets from a github repo: `/gh/OWNER/REPO[@TAG]/PATH`. For example:

```javascript
import tslib from "https://esm.sh/gh/microsoft/tslib"
```

or load a svg image from a github repo: https://esm.sh/gh/microsoft/fluentui-emoji/assets/Alien/Flat/alien_flat.svg

### Specify Dependencies

By default, esm.sh rewrites import specifiers based on the package dependencies. To specify the version of these dependencies, you can add the `?deps=PACKAGE@VERSION` query. To specify multiple dependencies, separate them with a comma, like this: `?deps=react@17.0.2,react-dom@17.0.2`.

```javascript
import React from "https://esm.sh/react@17.0.2"
import useSWR from "https://esm.sh/swr?deps=react@17.0.2"
```

### Specify External Dependencies

You can add the `?external=foo,bar` query to specify external dependencies.
Since these dependencies are not resolved, you need to use [**import maps**](https://github.com/WICG/import-maps) to specify the URL for these dependencies. If you are using Deno, you can use the [CLI Script](#using-cli-script) to generate and update the import maps that will resolve the external dependencies automatically.

```json
{
  "imports": {
    "preact": "https://esm.sh/preact@10.7.2",
    "preact-render-to-string": "https://esm.sh/preact-render-to-string@5.2.0?external=preact",
  }
}
```

Alternatively, you can **mark all dependencies as external** by adding a `*` prefix before the package name:

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

Import maps supports [**trailing slash**](https://github.com/WICG/import-maps#packages-via-trailing-slashes) that can not work with URL search params friendly. To fix this issue, esm.sh provides a special format for import URL that allows you to use query params with trailing slash: change the query prefix `?` to `&` and put it after the package version.

```json
{
  "imports": {
    "react-dom": "https://esm.sh/react-dom@18.2.0?pin=v120&dev",
    "react-dom/": "https://esm.sh/react-dom@18.2.0&pin=v120&dev/",
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

By default, esm.sh exports a module with all its exported members. However, if you want to import only a specific set of members, you can specify them by adding a `?exports=foo,bar` query to the import statement.

```javascript
import { __await, __rest } from "https://esm.sh/tslib" // 7.3KB
import { __await, __rest } from "https://esm.sh/tslib?exports=__await,__rest" // 489B
```

By using this feature, you can take advantage of tree shaking with esbuild and achieve a smaller bundle size. **Note** that this feature is only supported for ESM modules and not CJS modules.

### Bundle Mode

```javascript
import { Button } from "https://esm.sh/antd?bundle"
```

In **bundle** mode, all dependencies are bundled into a single JS file except the peer dependencies.

### Development Mode

```javascript
import React from "https://esm.sh/react?dev"
```

With the `?dev` option, esm.sh builds a module with `process.env.NODE_ENV` set to `"development"` or based on the condition `development` in the `exports` field of `package.json`. This is useful for libraries that have different behavior in development and production. For example, React will use a different warning message in development mode.

### ESBuild Options

By default, esm.sh checks the `User-Agent` header to determine the build target. You can also specify the `target` by adding `?target`, available targets are: **es2015** - **es2022**, **esnext**, **deno**, and **denonext**.

```javascript
import React from "https://esm.sh/react?target=es2020"
```

Other supported options of esbuild:

- [Conditions](https://esbuild.github.io/api/#conditions)
  ```javascript
  import foo from "https://esm.sh/foo?conditions=custom1,custom2"
  ```
- [Keep names](https://esbuild.github.io/api/#keep-names)
  ```javascript
  import foo from "https://esm.sh/foo?keep-names"
  ```
- [Ignore annotations](https://esbuild.github.io/api/#ignore-annotations)
  ```javascript
  import foo from "https://esm.sh/foo?ignore-annotations"
  ```

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

### Importing WASM Modules

esm.sh supports importing wasm modules in JS directly, to do that, you need to add `?module` query to the import URL:

```javascript
import wasm from "https://esm.sh/@dqbd/tiktoken@1.0.3/tiktoken_bg.wasm?module"

const { exports } = WebAssembly.instantiate(wasm, imports)
```

### Specify CJS Exports

If you get an error like `...not provide an export named...`, that means esm.sh can not resolve CJS exports of the module correctly. You can add `?cjs-exports=foo,bar` query to specify the export names:

```javascript
import { NinetyRing, NinetyRingWithBg } from "https://esm.sh/react-svg-spinners@0.3.1?cjs-exports=NinetyRing,NinetyRingWithBg"
```

## Building a Module with Custom Input(code)

This is an **_experimental_** API that allows you to build a module with custom input(code).

- Imports NPM/GH packages
- Supports TS/JSX syntaxes
- Bundle mulitple modules into a single JS file

```javascript
import build, { esm } from "https://esm.sh/build"

// use `esm` tag function
const ret = await esm`
/* @jsx h */
import { h } from "preact@10.13.2";
import { renderToString } from "preact-render-to-string@6.0.2";
export function render(): string {
  return renderToString(<h1>Hello world!</h1>);
}
`

// use `build` function
const ret = await build({
  dependencies: {
    "preact": "^10.13.2",
    "preact-render-to-string": "^6.0.2"
  },
  code: `
    /* @jsx h */
    import { h } from "preact";
    import { renderToString } from "preact-render-to-string";
    export function render(): string {
      return renderToString(<h1>Hello world!</h1>);
    }
  `,
  // for deno types checking
  types:`
    export function render(): string;
  `
})

// import module
const { render } = await import(ret.url)
// import bundled module
const { render } = await import(ret.bundleUrl)

render() // "<h1>Hello world!</h1>"
```

## Deno Compatibility

esm.sh is a **Deno-friendly** CDN that resolves Node's built-in modules (such as **fs**, **os**, etc.), making it compatible with Deno.

```javascript
import express from "https://esm.sh/express";

const app = express();
app.get("/", (req, res) => {
  res.send("Hello World");
});
app.listen(3000)
```

For users using deno `< 1.31`, esm.sh uses [deno.land/std@0.177.0/node](https://deno.land/std@0.177.0/node) as node compatibility layer. You can specify a different version by adding the `?deno-std=$VER` query:

```javascript
import postcss from "https://esm.sh/postcss?deno-std=0.128.0"
```

### X-Typescript-Types Header

Deno supports type definitions for modules with a `types` field in their `package.json` file through the `X-TypeScript-Types` header. This makes it possible to have type checking and auto-completion when using those modules in Deno. ([link](https://deno.land/manual/typescript/types#using-x-typescript-types-header)).


![Figure #1](./server/embed/assets/sceenshot-deno-types.png)

In case the type definitions provided by the `X-TypeScript-Types` header are incorrect, you can disable it by adding the `?no-dts` query to the module import URL:

```javascript
import unescape from "https://esm.sh/lodash/unescape?no-dts"
```

This will prevent the `X-TypeScript-Types` header from being included in the network request, and you can manually specify the types for the imported module.

### Using CLI Script

**esm.sh** provides a CLI script for managing imports with import maps in [Deno](https://deno.land). This CLI script automatically resolves dependencies and uses a pinned build version for stability.

To use the esm.sh CLI script, you first need to run the `init` command in your project's root directory:

```bash
deno run -A -r https://esm.sh init
```

Once you've initialized the script, you can use the following commands to manage your imports:

```bash
# Adding packages
deno task esm:add react react-dom     # add multiple packages
deno task esm:add react@17.0.2        # specify version
deno task esm:add react:preact/compat # using alias

# Updating packages
deno task esm:update react react-dom  # update specific packages
deno task esm:update                  # update all packages

# Removing packages
deno task esm:remove react react-dom
```

## Pinning Build Version

To ensure stable and consistent behavior, you may want to pin the build version of a module you're using from esm.sh. This helps you avoid potential breaking changes in the module caused by updates to the esm.sh server.

The `?pin` query allows you to specify a specific build version of a module, which is an **immutable** cached version stored on the esm.sh CDN.

```javascript
import React from "https://esm.sh/react-dom?pin=v120"
// or use version prefix
import React from "https://esm.sh/v120/react-dom"
```

By using the `?pin` query in the import statement, you can rest assured that the version of the module you're using will not change, even if updates are pushed to the esm.sh server. This helps ensure the stability and reliability of your application.

For UI libraries like _React_ and _Vue_, esm.sh uses a special build version `stable` to ensure single version of the library is used in the whole application.

## Global CDN

<img width="150" align="right" src="./server/embed/assets/cf.svg">

The Global CDN of esm.sh is provided by [Cloudflare](https://cloudflare.com), one of the world's largest and fastest cloud network platforms.

## Self-Hosting

To host esm.sh by yourself, check the [hosting](./HOSTING.md) documentation.

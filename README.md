![esm.sh](./server/embed/assets/og-image.svg)

<p align="left">
  <a href= "https://github.com/esm-dev/esm.sh/pkgs/container/esm.sh"><img src="https://img.shields.io/github/v/tag/esm-dev/esm.sh?label=Docker&display_name=tag&style=flat&colorA=232323&colorB=232323&logo=docker&logoColor=eeeeee" alt="Docker"></a>
  <a href="https://discord.gg/XDbjMeb7pb"><img src="https://img.shields.io/discord/1097820016893763684?style=flat&colorA=232323&colorB=232323&label=Discord&logo=&logoColor=eeeeee" alt="Discord"></a>
  <a href="https://github.com/sponsors/ije"><img src="https://img.shields.io/github/sponsors/ije?label=Sponsors&style=flat&colorA=232323&colorB=232323&logo=&logoColor=eeeeee" alt="Sponsors"></a>
</p>

# esm.sh

A global, fast & smart content delivery network(CDN) for modern(es2015+) web development.

## How to Use

esm.sh allows you to import [JavaScript ES modules](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Guide/Modules) on [NPM](https://npmjs.com), [GitHub](https://github.com), and [JSR](https://jsr.io) with a simple URL. **No installation/build steps needed.**

```js
import * as mod from "https://esm.sh/PKG[@SEMVER][/PATH]";
```

With [import maps](https://github.com/WICG/import-maps), you can even use bare import specifiers intead of URLs:

```html
<script type="importmap">
{
  "imports": {
    "react": "https://esm.sh/react@18.2.0"
    "react-dom/": "https://esm.sh/react-dom@18.2.0/"
  }
}
</script>
<script type="module">
  import React from "react"; // → https://esm.sh/react@18.2.0
  import { render } from "react-dom/client"; // → https://esm.sh/react-dom@18.2.0/client
</script>
```

> More details about the import map usage can be found in the [**Using Import Maps**](#using-import-maps) section.

### Supported Registries

- **[NPM](https://npmjs.com)**:
  ```js
  // Examples
  import React from "https://esm.sh/react"; // latest
  import React from "https://esm.sh/react@17"; // 17.0.2
  import React from "https://esm.sh/react@beta"; // latest beta
  import { renderToString } from "https://esm.sh/react-dom/server"; // sub-modules
  ```
- **[JSR](https://jsr.io)** (starts with `/jsr/`):
  ```js
  // Examples
  import { encodeBase64 } from "https://esm.sh/jsr/@std/encoding@1.0.0/base64";
  import { Hono } from "https://esm.sh/jsr/@hono/hono@4";
  ```
- **[GitHub](https://github.com)** (starts with `/gh/`):
  ```js
  // Examples
  import tslib from "https://esm.sh/gh/microsoft/tslib"; // latest
  import tslib from "https://esm.sh/gh/microsoft/tslib@2.6.0"; // the version '2.6.0' is a git tag
  ```

### Specifying Dependencies

By default, esm.sh rewrites import specifiers based on the package dependencies. To specify the version of these
dependencies, you can add the `?deps=PACKAGE@VERSION` query. To specify multiple dependencies, separate them with a
comma, like this: `?deps=react@17.0.2,react-dom@17.0.2`.

```js
import React from "https://esm.sh/react@17.0.2";
import useSWR from "https://esm.sh/swr?deps=react@17.0.2";
```

### Aliasing Dependencies

```js
import useSWR from "https://esm.sh/swr?alias=react:preact/compat";
```

in combination with `?deps`:

```js
import useSWR from "https://esm.sh/swr?alias=react:preact/compat&deps=preact@10.5.14";
```

### Tree Shaking

By default, esm.sh exports a module with all its exported members. However, if you want to import only a specific set of
members, you can specify them by adding a `?exports=foo,bar` query to the import statement.

```js
import { __await, __rest } from "https://esm.sh/tslib"; // 7.3KB
import { __await, __rest } from "https://esm.sh/tslib?exports=__await,__rest"; // 489B
```

By using this feature, you can take advantage of tree shaking with esbuild and achieve a smaller bundle size. **Note**
that this feature is only supported for ESM modules and not CJS modules.

### Bundling Strategy

By default, esm.sh bundles sub-modules that ain't declared in the `exports` field.

Bundling sub-modules can reduce the number of network requests for performance. However, it may bundle shared modules
repeatedly. In extreme case, it may break the side effects of the package, or change the `import.meta.url` semantics. To
avoid this, you can add `?bundle=false` to disable the default bundling behavior:

```js
import "https://esm.sh/@pyscript/core?bundle=false";
```

For package authors, you can override the bundling strategy by adding the `esm.sh` field to `package.json`:

```jsonc
{
  "name": "foo",
  "esm.sh": {
    "bundle": false // disables the default bundling behavior
  }
}
```

esm.sh also supports `?bundle=all` query to bundle the module with all external dependencies(except in `peerDependencies`) into a single JS file.

```js
import { Button } from "https://esm.sh/antd?bundle=all";
```

### Development Mode

```js
import React from "https://esm.sh/react?dev";
```

With the `?dev` option, esm.sh builds a module with `process.env.NODE_ENV` set to `"development"` or based on the
condition `development` in the `exports` field. This is useful for libraries that have different behavior in development
and production. For example, React uses a different warning message in development mode.

### ESBuild Options

By default, esm.sh checks the `User-Agent` header to determine the build target. You can also specify the `target` by
adding `?target`, available targets are: **es2015** - **es2022**, **esnext**, **deno**, **denonext**, and **node**.

```js
import React from "https://esm.sh/react?target=esnext";
```

Other supported options of esbuild:

- [Conditions](https://esbuild.github.io/api/#conditions)
  ```js
  import foo from "https://esm.sh/foo?conditions=custom1,custom2";
  ```
- [Keep names](https://esbuild.github.io/api/#keep-names)
  ```js
  import foo from "https://esm.sh/foo?keep-names";
  ```
- [Ignore annotations](https://esbuild.github.io/api/#ignore-annotations)
  ```js
  import foo from "https://esm.sh/foo?ignore-annotations";
  ```

### Web Worker

esm.sh supports `?worker` query to load the module as a web worker:

```js
import createWorker from "https://esm.sh/monaco-editor/esm/vs/editor/editor.worker?worker";

// create a worker
const worker = createWorker();
// rename the worker by adding the `name` option for debugging
const worker = createWorker({ name: "editor.worker" });
// inject code into the worker
const worker = createWorker({ inject: "self.onmessage = (e) => self.postMessage(e.data)" });
```

You can import any module as a worker from esm.sh with the `?worker` query. Plus, you can access the module's exports in the
`inject` code. For example, uing the `xxhash-wasm` to hash strings in a worker:

```js
import createWorker from "https://esm.sh/xxhash-wasm@1.0.2?worker";

// variable '$module' is the imported 'xxhash-wasm' module
const inject = `
const { default: xxhash } = $module
self.onmessage = async (e) => {
  const hasher = await xxhash()
  self.postMessage(hasher.h64ToString(e.data))
}
`;
const worker = createWorker({ inject });
worker.onmessage = (e) => console.log("hash is", e.data);
worker.postMessage("The string that is being hashed");
```

> Note: The `inject` parameter must be a valid JavaScript code, and it will be executed in the worker context.

### Package CSS

```html
<link rel="stylesheet" href="https://esm.sh/monaco-editor?css">
```

This only works when the package **imports CSS files in JS** directly.

### Importing WASM as Module

esm.sh supports importing wasm modules in JS directly, to do that, you need to add `?module` query to the import URL:

```js
import wasm from "https://esm.sh/@dqbd/tiktoken@1.0.3/tiktoken_bg.wasm?module";

const { exports } = new WebAssembly.Instance(wasm, imports);
```

> Note: The `*.wasm?module` pattern requires the [top-level-await](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Operators/await#top_level_await) feature to be supported by the browser.

## Using Import Maps

[**Import Maps**](https://github.com/WICG/import-maps) has been supported by most modern browsers and Deno natively.
This allows _**bare import specifiers**_, such as `import React from "react"`, to work.

esm.sh introduces the `?external` for specifying external dependencies. By employing this query, esm.sh maintains the import specifier intact, leaving it to the browser/Deno to resolve based on the import map. For example:

```html
<script type="importmap">
{
  "imports": {
    "preact": "https://esm.sh/preact@10.7.2",
    "preact/": "https://esm.sh/preact@10.7.2/",
    "preact-render-to-string": "https://esm.sh/preact-render-to-string@5.2.0?external=preact"
  }
}
</script>
<script type="module">
  import { h } from "preact";
  import { useState } from "preact/hooks";
  import { render } from "preact-render-to-string";
</script>
```

Alternatively, you can **mark all dependencies as external** by adding a `*` prefix before the package name:

```json
{
  "imports": {
    "preact": "https://esm.sh/preact@10.7.2",
    "preact-render-to-string": "https://esm.sh/*preact-render-to-string@5.2.0",
    "swr": "https://esm.sh/*swr@1.3.0",
    "react": "https://esm.sh/preact@10.7.2/compat"
  }
}
```

Import maps supports [**trailing slash**](https://github.com/WICG/import-maps#packages-via-trailing-slashes) that can
not work with URL search params friendly. To fix this issue, esm.sh provides a special format for import URL that allows
you to use query params with trailing slash: change the query prefix `?` to `&` and put it after the package version.

```json
{
  "imports": {
    "react-dom": "https://esm.sh/react-dom@18.2.0?dev",
    "react-dom/": "https://esm.sh/react-dom@18.2.0&dev/"
  }
}
```

## Escape Hatch: Raw Source Files

In rare cases, you may want to request JS source files from packages, as-is, without transformation into ES modules. To
do so, you need to add a `?raw` query to the request URL.

```html
<script src="https://esm.sh/p5/lib/p5.min.js?raw"></script>
```

> You may alternatively use `raw.esm.sh/<PATH>` as the origin, which is equivalent to `esm.sh/<PATH>?raw`,
> that transitive references in the raw assets will also be raw requests.

## Deno Compatibility

esm.sh is a **Deno-friendly** CDN that resolves Node's built-in modules (such as **fs**, **os**, **net**, etc.), making
it compatible with Deno.

```js
import express from "https://esm.sh/express";

const app = express();
app.get("/", (req, res) => {
  res.send("Hello World");
});
app.listen(3000);
```

Deno supports type definitions for modules with a `types` field in their `package.json` file through the
`X-TypeScript-Types` header. This makes it possible to have type checking and auto-completion when using those modules
in Deno. ([link](https://deno.land/manual/typescript/types#using-x-typescript-types-header)).

![Figure #1](./server/embed/assets/sceenshot-x-typescript-types.png)

In case the type definitions provided by the `X-TypeScript-Types` header is incorrect, you can disable it by adding the
`?no-dts` query to the module import URL:

```js
import unescape from "https://esm.sh/lodash/unescape?no-dts";
```

This will prevent the `X-TypeScript-Types` header from being included in the network request, and you can manually
specify the types for the imported module.

## Supporting Nodejs/Bun

Nodejs(18+) supports http importing under the `--experimental-network-imports` flag. Bun doesn't support http modules
yet.

We highly recommend [Reejs](https://ree.js.org/) as the runtime with esm.sh that works both in Nodejs and Bun.

## Global CDN

<img width="150" align="right" src="./server/embed/assets/cf-logo.svg" />

The Global CDN of esm.sh is provided by [Cloudflare](https://cloudflare.com), one of the world's largest and fastest
cloud network platforms.

## Self-Hosting

To host esm.sh by yourself, check the [hosting](./HOSTING.md) documentation.

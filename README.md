# ESM

A fast, global content delivery network for ES Modules. All modules are transformed to ESM by [esbuild](https://github.com/evanw/esbuild) in [NPM](http://npmjs.org/).

## Import from URL

```javascript
import React from 'https://esm.sh/react'
```

### Specify version

```javascript
import React from 'https://esm.sh/react@17.0.1'
```

### Submodule

```javascript
import { renderToString } from 'https://esm.sh/react-dom/server'
```

or import non-module(js) files:

```javascript
import 'https://esm.sh/tailwindcss/dist/tailwind.min.css'
```

### Specify ESM target

```javascript
import React from 'https://esm.sh/react?target=es2020'
```

Avaiable `target`: **es2015** - **es2020**, **esnext**

### Development mode

```javascript
import React from 'https://esm.sh/react?dev'
```

### Bundle mode

```javascript
import React from 'https://esm.sh/[react,react-dom,swr]/react'
import ReactDom from 'https://esm.sh/[react,react-dom,swr]/react-dom'
```

or your can define the bundle list in `import-map.json` ([import-maps proposal](https://github.com/WICG/import-maps))

```json
{
    "imports": {
        "https://esm.sh/": "https://esm.sh/[react,react-dom,swr]/",
        ...
    }
}
```

```javascript
import React from 'https://esm.sh/react' // actual from 'https://esm.sh/[react,react-dom,swr]/react'
```

⚠️ The bundling packages in URL are litmited up to **10**, to bundle more packages, please use the **esm** client(WIP).

## Deno compatibility

**esm.sh** provides polyfills for the node internal modules(**fs**, **os**, etc) with [`deno.land/std/node`](https://deno.land/std/node) to support some packages working in Deno, like `postcss`:

```javascript
import postcss from 'https://esm.sh/postcss'
import autoprefixer from 'https://esm.sh/autoprefixer'

const css = (await postcss([ autoprefixer]).process(`
    backdrop-filter: blur(5px);
    user-select: none;
`).async()).content
```

### X-Typescript-Types

By default, **esm.sh** will response a custom HTTP header of `X-TypeScript-Types` when the types(dts) defined, that is useful for deno types check ([link](https://deno.land/manual/getting_started/typescript#x-typescript-types-custom-header)).

![figure #1](./assets/figure-1.png)

You can pass the `no-check` query to disable the `types` header if some types are incorrect:

```javascript
import unescape from 'https://esm.sh/lodash/unescape?no-check'
```

## Caveats

Different with [Skypack](https://skypack.dev) and [jspm](https://jspm.org), **esm.sh** will bundle all dependencies(exclude peerDependencies) for each package, that means there may be redundant contents transmitted when you are importing multiple packages.<br>
This should be improved when the http/3(quic) is ready. For now the best practice is using the **bundle mode**.

As optional, you can split code manually with `external` query:

```javascript
import React from 'https://esm.sh/react@16.14.0'
import useSWR from 'https://esm.sh/swr?external=react@16.14.0'
```

## Network of esm.sh
- Main server in HK
- Global CDN by [Cloudflare](https://cloudflare.com)
- China CDN by [Aliyun](https://aliyun.com)

## Self-Hosting

You will need [Go](https://golang.org/dl) 1.14+ to compile the server, and ensure [supervisor](http://supervisord.org/) installed on your host machine.<br>
The server runtime will install the latest nodejs (14+ LTS) automatically.

```bash
$ git clone https://github.com/postui/esm.sh
$ cd esm.sh
$ sh ./scripts/deploy.sh
```

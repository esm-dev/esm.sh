# ESM

A fast, global content delivery network for ES Modules. All modules in [NPM](http://npmjs.org/) are transformed to ESM by [esbuild](https://github.com/evanw/esbuild).

## Import from URL

```javascript
import React from 'https://esm.sh/react'
```

### Specify version

```javascript
import React from 'https://esm.sh/react@17.0.2'
```

or use a major version:

```javascript
import React from 'https://esm.sh/react@17'
```

### Submodule

```javascript
import { renderToString } from 'https://esm.sh/react-dom/server'
```

or import non-module(js) files:

```javascript
import 'https://esm.sh/tailwindcss/dist/tailwind.min.css'
```

### Bundle mode

```javascript
import { Button } from 'https://esm.sh/antd?bundle'
```

In **bundle** mode, all dependencies will be bundled into one JS file.

### Development mode

```javascript
import React from 'https://esm.sh/react?dev'
```

### Specify external deps

```javascript
import React from 'https://esm.sh/react@16.14.0'
import useSWR from 'https://esm.sh/swr?deps=react@16.14.0'
```

Separate multiple deps with commas: `?deps=react@16.14.0,react-dom@16.14.0`

### Alias dependencies

```javascript
import useSWR from 'https://esm.sh/swr?alias=react:preact/compat'
```

in combination with `?deps`:

```javascript
import useSWR from 'https://esm.sh/swr?alias=react:preact/compat&deps=preact@10.5.14'
```

This idea was from [@lucacasonato](https://github.com/lucacasonato).

### Specify ESM target

```javascript
import React from 'https://esm.sh/react?target=es2020'
```

By default, esm.sh will check the browser's `User-Agent` to get the build target automatically. You also can specify it with the `target` query. Available targets: **es2015** - **es2021**, **esnext**, **node**, and **deno**.

### Package CSS

```javascript
import Daygrid from 'https://esm.sh/@fullcalendar/daygrid'
```

```html
<link rel="stylesheet" href="https://esm.sh/@fullcalendar/daygrid?css">
```

Only works when you import a NPM package that imports css files directly.

## Deno compatibility

**esm.sh** will resolve the node internal modules (**fs**, **os**, etc.) with [`deno.land/std/node`](https://deno.land/std/node) to support some packages working in Deno, like `postcss`:

```javascript
import postcss from 'https://esm.sh/postcss'
import autoprefixer from 'https://esm.sh/autoprefixer'

const { css } = await postcss([ autoprefixer ]).process(`
  backdrop-filter: blur(5px);
  user-select: none;
`).async() 
console.log(css)
```

### X-Typescript-Types

By default, **esm.sh** will respond with a custom `X-TypeScript-Types` HTTP header when types (`.d.ts`) are defined. This is useful for deno type checks ([link](https://deno.land/manual/typescript/types#using-x-typescript-types-header)).

![figure #1](./embed/assets/sceenshot-deno-types.png)

You can pass the `no-check` query to disable the `X-TypeScript-Types` header if some types are incorrect:

```javascript
import unescape from 'https://esm.sh/lodash/unescape?no-check'
```

## Network of esm.sh
- Main server in HK
- Global CDN by [Cloudflare](https://cloudflare.com)
- China CDN by [Aliyun](https://aliyun.com) (use [mmdb_china_ip_list](https://github.com/alecthw/mmdb_china_ip_list) to split traffic)

## Self-Hosting

You will need [Go](https://golang.org/dl) 1.16+ to compile the server, and ensure [supervisor](http://supervisord.org/) installed on your host machine.<br>
The server runtime will install the nodejs (14 LTS) automatically.

```bash
$ git clone https://github.com/postui/esm.sh
$ cd esm.sh
$ sh ./scripts/deploy.sh
```

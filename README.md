# ESM
A fast, global content delivery network and package manager for ES Modules. All modules are transformed to ESM by [esbuild](https://github.com/evanw/esbuild) from [npm](http://npmjs.org/).

## Import from URL
```javascript
import React from 'https://esm.sh/react'
```

### Specify version
```javascript
import React from 'https://esm.sh/react@16.13.1'
```

### Submodule
```javascript
import { renderToString } from 'https://esm.sh/react-dom/server'
```

### Specify ESM target
```javascript
import React from 'https://esm.sh/react?target=es2020'
```

### Development mode
```javascript
import React from 'https://esm.sh/react?dev'
```

### Bundle mode
```javascript
import React from 'https://esm.sh/[react,react-dom]/react'
import ReactDom from 'https://esm.sh/[react,react-dom]/react-dom'
```

or your can define bundle list in the `import-map.json` ([import-maps proposal](https://github.com/WICG/import-maps))
```json
{
    "imports": {
        "https://esm.sh/": "https://esm.sh/[react,react-dom]/",
        ...
    }
}
```

```javascript
import React from 'https://esm.sh/react' // actual from 'https://esm.sh/[react,react-dom]/react'
```

⚠️ The bundling packages in URL are litmited up to **10**, to bundle more packages, please use esm client.


## ESM Client in Deno [WIP]
```bash
# install esm command
deno install --allow-read --allow-write --allow-net -n esm https://deno.land/x/esm/cli.ts

# add some modules
$ esm add react react-dom

# specify version or tag
$ esm add react@16.13.1
$ esm add react@next

# remove some modules
$ esm remove lodash

# update installed modules to latest version
$ esm update

# help message
$ esm -h
```

## Caveat
Different with [Skypack](https://skypack.dev) and [jspm](https://jspm.org), **esm.sh** will bundle all dependencies(exclude peerDependencies) for each packages, that means there may be redundant contents transmitted when you are importing multiple packages.<br>
This should be improved when the http/3(quic) is ready. For now the best practice is using the **bundle mode**.

## Self-Hosting

You will need [Go](https://golang.org/dl) 1.14+ to compile the server, and ensure the [supervisor](http://supervisord.org/) installed on your host machine.<br>
The server runtime will check the nodejs installation (12+) exists or install the latest LTS version automatically.

```bash
$ git clone https://github.com/postui/esm.sh
$ cd esm.sh
$ sh ./scripts/deploy.sh
```

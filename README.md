# ESM Delivery
A fast, global content delivery network for ES Modules. All modules are transformed to ESM by [esbuild](https://github.com/evanw/esbuild) from [npm](http://npmjs.org/).

# Usage
```javascript
import React from 'https://esm.sh/react'
```

### Specify version
```javascript
import React from 'https://esm.sh/react@16.13.1'
```

### Bundle mode
```javascript
// bundle multiple packages in a single file  
import React from 'https://esm.sh/react?bundle=react,react-dom'
import ReactDom from 'https://esm.sh/react-dom?bundle=react,react-dom'
```

### Specify ESM target
```javascript
import React from 'https://esm.sh/react?target=es2020'
```

### Development mode
```javascript
import React from 'https://esm.sh/react?env=development'
```

### Submodule
```javascript
import { renderToString } from 'https://esm.sh/react-dom/server'
```

# Self-Hosting
You will need [Go](https://golang.org/dl) 1.5+ to compile the server application. On the host ensure the [supervisor](http://supervisord.org/) installed, then run `sh ./scripts/deploy.sh` to deploy the server application. The server application will check the nodejs installation (12+) or install the latest LTS version automatically.

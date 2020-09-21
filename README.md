# ESM Delivery
A fast, global content delivery network for ES Modules.

```javascript
import React from 'https://esm.sh/react'
```

## Bundle Mode

```javascript
import React from 'https://esm.sh/react?bundle=react,react-dom'
import ReactDom from 'https://esm.sh/react-dom?bundle=react,react-dom'
```

## Specify ESM Target
```javascript
import React from 'https://esm.sh/react?target=es2020'
```

## Development Mode

```javascript
import React from 'https://esm.sh/react?env=development'
```

## Submodule

```javascript
import { renderToString } from 'https://esm.sh/react-dom/server'
```

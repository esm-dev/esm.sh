# ESM Delivery
A fast, global content delivery network for ES Modules.

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
// bundle React and ReactDom in a single file  
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

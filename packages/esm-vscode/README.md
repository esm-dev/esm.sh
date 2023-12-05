![Figure #1](https://esm.sh/embed/assets/sceenshot-deno-types.png)

# esm.sh for VS Code

An extension automatically loads types from [esm.sh](https://esm.sh) CDN for JavaScript and TypeScript files. No `npm install` required.

## Usage

This extension respects `importmap` script tag in `index.html` of your project root. With [import maps](https://github.com/WICG/import-maps), you can use "bare import specifiers", such as `import React from "react"`, to work.

```html
<!-- index.html -->

<!DOCTYPE html>
<script type="importmap">
  {
    "imports": {
      "@jsxImportSource": "https://esm.sh/react@18.2.0",
      "react": "https://esm.sh/react@18.2.0",
    }
  }
</script>
```

```jsx
// app.jsx

import { useState } from "react";

export default function App() {
  return <h1>Hello World!</h1>;
}
```

> The `@jsxImportSource` is a special field for jsx runtime types.

> A "esm.sh: Add Module" command is also provided to add a module to the import map.

---
name: A failing module import in Deno
about: Submit a report if a module fails to import in Deno.
title: 'Failed to import -'
labels: deno
---

## Failing module

- **GitHub**: https://github.com/my/repo
- **npm**: https://npmjs.com/package/my_package

```js
import { something } from "https://esm.sh/my_module"
```

## Error message

After running `deno run` I got this:

```
/* your error log here */
```

## Additional info

- **Deno version**:

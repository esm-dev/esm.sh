---
name: A failing module import
about: Submit a report if a module fails to import
title: 'Failed to import -'
labels: bug
assignees: ''
---

## Failing module

- **GitHub**: 
- **npm**:

```js
import { something } from 'https://esm.sh/my_module'
```

## Error message

After running `deno run` I get this:

```
/* your error log here */
```

## Additional info

- **esm.sh version**: v47
- **Deno version**: 1.13.2

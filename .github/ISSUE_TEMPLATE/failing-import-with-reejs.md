---
name: A failing module import in Nodejs/Bun/Deno (via Reejs)
about: Submit a report if a module fails to import in Any Supported Runtime with Reejs' URL Imports.
title: 'Failed to import -'
labels: reejs
---

## Failing module

- **GitHub**: https://github.com/my/repo
- **npm**: https://npmjs.com/package/my_package

```js
let package = await URLImport("https://esm.sh/my_module");
```

## Error message

After running the code in `reejs repl` I got this:

```
/* your error log here */
```

## Additional info

- **esm.sh version**:
- **Paste the `reejs doctor` report:

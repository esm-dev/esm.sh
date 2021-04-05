# Change Log

## V35

- Set build `target` by the `user-agent` of browser automaticlly

## V34

- **&middot; Breaking** Remove bundle mode
- Add build queue instead of mutex lock
- Use AST([cjs-module-lexer](https://github.com/guybedford/cjs-module-lexer)) to parse cjs exports
- Add a testing page at https://esm.sh?test
- **&middot; Fix** `__setImmediate$` is not defined
- **&middot; Fix** Support exports define in package.json
- **&middot; Fix** Support mjs extension
- **&middot; Fix** Improve NpmPackage resolve (#41)
- Upgrade esbuild to **0.11.4**
- Upgrade rex to **1.3.0**

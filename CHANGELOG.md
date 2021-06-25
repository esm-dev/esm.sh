# Change Log

## V43

- Add `/status.json` api
- Use previous build instead of waiting/404 (fix #74)
- Fix deps query (#71)

## V42

- Add `__esModule` reserved word
- Align require change for esbuild 0.12
- Fix setImmediate polyfill args (#75)
- Upgrade **esbuild** to **0.11.12**

## V41

- Add `timeout` (30 seconds) for new build request, or use previous build version instead if it exists
- Fix `bundle` mode
- Fix build dead loop
- Upgrade **esbuild** to **0.11.12**

## V40

- Update polyfills for node builtin modules
- Upgrade **esbuild** to **0.11.9**

## V39

- Imporve `parseCJSModuleExports` to support json module
- Pass `NODE_ENV` to `parseCJSModuleExports`
- Update node buffer polyfill
- Upgrade postdb to **v0.6.2**

## V38

- Fix build for packages with `module` type ([#48](https://github.com/postui/esm.sh/issues/48))
- Improve `parseCJSModuleExports` function (use cjs-module-lexer and nodejs eval both to parse cjs exports, and ignore JSON module)
- Pass `NODE_ENV` to `parseCJSModuleExports` function
- Upgrade **esbuild** to **0.11.6**

## V37

- Add **bundle** mode
- Fix module exports parsing

## V36

- Fix esm build for some edge cases
- Add simple test (thanks @zhoukekestar)
- Upgrade esbuild to 0.11.5

## V35

- Set build `target` by the `user-agent` of browser automaticlly

## V34

- Remove bundle mode **&middot; Breaking**
- Add build queue instead of mutex lock
- Use AST([cjs-module-lexer](https://github.com/guybedford/cjs-module-lexer)) to parse cjs exports
- Add a testing page at https://esm.sh?test
- Fix `__setImmediate$` is not defined
- Support exports define in package.json
- Support mjs extension
- Improve NpmPackage resolve (**fix** #41)
- Upgrade esbuild to **0.11.4**
- Upgrade rex to **1.3.0**

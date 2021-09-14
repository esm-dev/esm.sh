# Change Log

## v51

- Fix build breaking change in v50 (fix [#131](https://github.com/postui/esm.sh/issues/131)).
- Add `localLRU` **FS** layer ([#126](https://github.com/postui/esm.sh/issues/126))
- Add a `Cache Interface` that is using to store temporary data like npm packages info.
- Do not try to build `/favicon.ico` ([#132](https://github.com/postui/esm.sh/issues/132))
- Add lovely `pixi.js`, `three.js` and `@material-ui/core` testing by @imisaacs ([#134](https://github.com/postui/esm.sh/issues/134), [#139](https://github.com/postui/esm.sh/issuGes/139)).

## v50

- Improve build performance to burn the server CPU cores! Before this, to build a module to ESM which has heavy deps maybe very slow since the single build task only uses one CPU core.
- Rewrite the **dts transformer** to get better deno types compatibility and faster transpile speed.
- Add Deno **testing CI** on Github.

## v49

- Improve the build process to fix an edge case reported in [#118](https://github.com/postui/esm.sh/issues/118)
	```js
	const Parser = require('htmlparser').Parser;
	```
	esm (v48) output:
	```js
	import htmlparser2 from '/v48/htmlparser2@5.0.0/es2021/htmlparser2.js'
	const Parser = htmlparser2.Parser; // parser is undefined
	```
	the expected output was fixed in v49:
	```js
	import { Parser as htmlparser2Parser } from '/v48/htmlparser2@5.0.0/es2021/htmlparser2.js'
	const Parser = htmlparser2Parser; // parser is a class
	```
- Add more polyfills for Deno, huge thanks to @talentlessguy ([#117](https://github.com/postui/esm.sh/issues/117))
  - path
  - querystring
  - url
  - timers
-	Better self-hosting options improved by @imisaacs, super! ([#116](https://github.com/postui/esm.sh/issues/116), [#119](https://github.com/postui/esm.sh/issues/116), [#120](https://github.com/postui/esm.sh/issues/120), [#122](https://github.com/postui/esm.sh/issues/122))
- Add **Unlimted(max 1PB) Storage** to store builds and cache via NFS on esm.sh back server behind Cloudflare

## v48

- Improve **cjs-lexer** service to handle the edge case is shown below:
	```js
	function debounce() {};
	debounce.debounce = debounce;
	module.exports = debounce;
	```
	esm output:
	```js
	export { debounce } // this was missed
	export default debounce
	```
- Ignore `?target` in Deno (fix [#109](https://github.com/postui/esm.sh/issues/109))
- Add **Storage Interface** to store data to anywhere (currently only support [postdb](https://github.com/postui/postdb) + local FS)

## v47

- Improve dts transformer to use cdn domain (fix [#104](https://github.com/postui/esm.sh/issues/104))
- Update polyfills (fix [#105](https://github.com/postui/esm.sh/issues/105))

## v46

- Split modules based on exports defines (ref [#78](https://github.com/postui/esm.sh/issues/78))
- Add `cache-folder` config for `yarn add`
- Improve `resolveVersion` to support format 4.x (fix [#93](https://github.com/postui/esm.sh/issues/93))
- Import initESM to support bare exports in package.json (fix [#97](https://github.com/postui/esm.sh/issues/97))
- Bundle mode should respect the extra external (fix [#98](https://github.com/postui/esm.sh/issues/98))
- Support node:path importing (fix [#100](https://github.com/postui/esm.sh/issues/100))
- Pass `?alias` and `?deps` to deps (fix [#101](https://github.com/postui/esm.sh/issues/101))
- Improve `cjs-lexer` sever (fix [#103](https://github.com/postui/esm.sh/issues/103))
- Upgrade **rex** to **1.4.1**
- Upgrade **esbuild** to **0.12.24**

## V45

- Improve build performance
- Filter `cjs-moudle-lexer` server invalid exports output
- Improve `resolveVersion` function to support format like **4.x** (fix [#93](https://github.com/postui/esm.sh/issues/93))
- Improve **dts** transform (fix [#95](https://github.com/postui/esm.sh/issues/95))

## V44

- Add `Alias` feature ([#89](https://github.com/postui/esm.sh/issues/89))
  ```javascript
  import useSWR from 'https://esm.sh/swr?alias=react:preact/compat'
  ```
  in combination with `?deps`:
  ```javascript
  import useSWR from 'https://esm.sh/swr?alias=react:preact/compat&deps=preact@10.5.14'
  ```
  The origin idea was came from [@lucacasonato](https://github.com/lucacasonato).
- Add `node` build target ([#84](https://github.com/postui/esm.sh/issues/84))
- Check `exports` field to get entry point in `package.json`
- Run cjs-lexer as a server
- Upgrade **esbuild** to **0.11.18** with `es2021` build target
- Bugfixs for
[#90](https://github.com/postui/esm.sh/issues/90),
[#85](https://github.com/postui/esm.sh/issues/85),
[#83](https://github.com/postui/esm.sh/issues/83),
[#77](https://github.com/postui/esm.sh/issues/77),
[#65](https://github.com/postui/esm.sh/issues/65),
[#48](https://github.com/postui/esm.sh/issues/48),
[#41](https://github.com/postui/esm.sh/issues/41).

## V43

- Add `/status.json` api
- Use previous build instead of waiting/404 (fix [#74](https://github.com/postui/esm.sh/issues/74))
- Fix deps query ([#71](https://github.com/postui/esm.sh/issues/71))

## V42

- Add `__esModule` reserved word
- Align require change for esbuild 0.12
- Fix setImmediate polyfill args ([#75](https://github.com/postui/esm.sh/issues/75))
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
- Improve NpmPackage resolve (**fix** [#41](https://github.com/postui/esm.sh/issues/41))
- Upgrade esbuild to **0.11.4**
- Upgrade rex to **1.3.0**

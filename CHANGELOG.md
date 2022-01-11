# Change Log

## v62

- bugfixs for [#240](https://github.com/esm-dev/esm.sh/issues/240),[#242](https://github.com/esm-dev/esm.sh/issues/242),[#248](https://github.com/esm-dev/esm.sh/issues/248)


## v61

- Support **React new JSX transforms** in Deno(1.16+) (close [#225](https://github.com/esm-dev/esm.sh/issues/225))

## v60

- bugfixs for [#244](https://github.com/esm-dev/esm.sh/issues/244),[#245](https://github.com/esm-dev/esm.sh/issues/245),[#246](https://github.com/esm-dev/esm.sh/issues/246)

## v59

- improve `cjs-esm-exports` to support some edge cases:
  ```js
	var foo = exports.foo || (exports.foo = {});
	((bar) => { ... })(exports.bar || (exports.bar = {}));
	```
- Fix `@types` verisoning:
  - `marked` -> `@types/marked@4.0.1`
  - `marked@2` -> `@types/marked@2.0.5`
  - `marked?dep=@types/marked@4.0.0` -> `@types/marked@4.0.0`
- Upgrade `deno.land/std/node` polyfill to **0.119.0**
- Upgrade `esbuild` to **v0.14.8**

## v58

- Recover the stable queue
- Filter invalid pathnames like `/wp-admin/login.php`
- Fix `?pin` mode when build failed (close [#206](https://github.com/esm-dev/esm.sh/issues/206))

## v57

- Add `?pin` mode 
- Improve build stability
- Fix `marked@4` import
- Fix invalid types hangs forever (close [#201](https://github.com/esm-dev/esm.sh/issues/201))

## v56
- `cjs-esm-exports` supports tslib `__exportStar` (close [#197](https://github.com/esm-dev/esm.sh/issues/197))
- Improve node `perf_hooks` polyfill
- Fix redeclared process polyfill (close [#195](https://github.com/esm-dev/esm.sh/issues/195))
- Fix `?worker` mode on deno ([#198](https://github.com/esm-dev/esm.sh/issues/198))
- Add `he` to `cjs-esm-exports` require mode allow list (close [#200](https://github.com/esm-dev/esm.sh/issues/200))
- Fix package css redirect link
- Upgrade **esbuild** to v0.13.12

## v55

- Add playground to write esm app online, try it on https://esm.sh?playground
- Add a better **cjs exports parser**: [cjs-esm-exports](https://www.npmjs.com/package/cjs-esm-exports)
- Support web worker
  ```js
  import editorWorker from '/monaco-editor/esm/vs/editor/editor.worker?worker'
  
	const worker = new editorWorker()
	```
-	Add `queue` interface
- Support **dataurl**, **.wasm** import
- Import deno polyfills from https://deno.land/std@0.113.0/node
- Fix package CSS

## v54

- Update deno polyfills from 0.106.0 to 0.110.0 ([#190](https://github.com/esm-dev/esm.sh/issues/190))
- Add deno `module` polyfill ([#164](https://github.com/esm-dev/esm.sh/issues/164))
- Fix (storage/fs_local) file path portability bug ([#158](https://github.com/esm-dev/esm.sh/issues/158))

## v53

- Add `Cache-Tag` header for CDN purge
- Add **s3** storage support ([#153](https://github.com/esm-dev/esm.sh/issues/153))
- Fix `require` replacement ([#154](https://github.com/esm-dev/esm.sh/issues/154))

## v52

- Fix types build ([#149](https://github.com/esm-dev/esm.sh/issues/149))
- Use `stream` and `events` from deno std/node ([#136](https://github.com/esm-dev/esm.sh/issues/148)) @talentlessguy
- Fix `localLRU` and allow for `memoryLRU` ([#148](https://github.com/esm-dev/esm.sh/issues/148)) @jimisaacs

## v51

- Fix build breaking change in v50 ([#131](https://github.com/esm-dev/esm.sh/issues/131)).
- Add `localLRU` **FS** layer ([#126](https://github.com/esm-dev/esm.sh/issues/126))
- Add a `Cache Interface` that is using to store temporary data like npm packages info.
- Do not try to build `/favicon.ico` ([#132](https://github.com/esm-dev/esm.sh/issues/132))
- Add lovely `pixi.js`, `three.js` and `@material-ui/core` testing by @jimisaacs ([#134](https://github.com/esm-dev/esm.sh/issues/134), [#139](https://github.com/esm-dev/esm.sh/issuGes/139)).

## v50

- Improve build performance to burn the server CPU cores! Before this, to build a module to ESM which has heavy deps maybe very slow since the single build task only uses one CPU core.
- Rewrite the **dts transformer** to get better deno types compatibility and faster transpile speed.
- Add Deno **testing CI** on Github.

## v49

- Improve the build process to fix an edge case reported in [#118](https://github.com/esm-dev/esm.sh/issues/118)
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
- Add more polyfills for Deno, huge thanks to @talentlessguy ([#117](https://github.com/esm-dev/esm.sh/issues/117))
  - path
  - querystring
  - url
  - timers
-	Better self-hosting options improved by @jimisaacs, super! ([#116](https://github.com/esm-dev/esm.sh/issues/116), [#119](https://github.com/esm-dev/esm.sh/issues/116), [#120](https://github.com/esm-dev/esm.sh/issues/120), [#122](https://github.com/esm-dev/esm.sh/issues/122))
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
- Ignore `?target` in Deno (fix [#109](https://github.com/esm-dev/esm.sh/issues/109))
- Add **Storage Interface** to store data to anywhere (currently only support [postdb](https://github.com/postui/postdb) + local FS)

## v47

- Improve dts transformer to use cdn domain (fix [#104](https://github.com/esm-dev/esm.sh/issues/104))
- Update polyfills (fix [#105](https://github.com/esm-dev/esm.sh/issues/105))

## v46

- Split modules based on exports defines (ref [#78](https://github.com/esm-dev/esm.sh/issues/78))
- Add `cache-folder` config for `yarn add`
- Improve `resolveVersion` to support format 4.x (fix [#93](https://github.com/esm-dev/esm.sh/issues/93))
- Import initESM to support bare exports in package.json (fix [#97](https://github.com/esm-dev/esm.sh/issues/97))
- Bundle mode should respect the extra external (fix [#98](https://github.com/esm-dev/esm.sh/issues/98))
- Support node:path importing (fix [#100](https://github.com/esm-dev/esm.sh/issues/100))
- Pass `?alias` and `?deps` to deps (fix [#101](https://github.com/esm-dev/esm.sh/issues/101))
- Improve `cjs-lexer` sever (fix [#103](https://github.com/esm-dev/esm.sh/issues/103))
- Upgrade **rex** to **1.4.1**
- Upgrade **esbuild** to **0.12.24**

## V45

- Improve build performance
- Filter `cjs-moudle-lexer` server invalid exports output
- Improve `resolveVersion` function to support format like **4.x** (fix [#93](https://github.com/esm-dev/esm.sh/issues/93))
- Improve **dts** transform (fix [#95](https://github.com/esm-dev/esm.sh/issues/95))

## V44

- Add `Alias` feature ([#89](https://github.com/esm-dev/esm.sh/issues/89))
  ```javascript
  import useSWR from 'https://esm.sh/swr?alias=react:preact/compat'
  ```
  in combination with `?deps`:
  ```javascript
  import useSWR from 'https://esm.sh/swr?alias=react:preact/compat&deps=preact@10.5.14'
  ```
  The origin idea was came from [@lucacasonato](https://github.com/lucacasonato).
- Add `node` build target ([#84](https://github.com/esm-dev/esm.sh/issues/84))
- Check `exports` field to get entry point in `package.json`
- Run cjs-lexer as a server
- Upgrade **esbuild** to **0.11.18** with `es2021` build target
- Bugfixs for
[#90](https://github.com/esm-dev/esm.sh/issues/90),
[#85](https://github.com/esm-dev/esm.sh/issues/85),
[#83](https://github.com/esm-dev/esm.sh/issues/83),
[#77](https://github.com/esm-dev/esm.sh/issues/77),
[#65](https://github.com/esm-dev/esm.sh/issues/65),
[#48](https://github.com/esm-dev/esm.sh/issues/48),
[#41](https://github.com/esm-dev/esm.sh/issues/41).

## V43

- Add `/status.json` api
- Use previous build instead of waiting/404 (fix [#74](https://github.com/esm-dev/esm.sh/issues/74))
- Fix deps query ([#71](https://github.com/esm-dev/esm.sh/issues/71))

## V42

- Add `__esModule` reserved word
- Align require change for esbuild 0.12
- Fix setImmediate polyfill args ([#75](https://github.com/esm-dev/esm.sh/issues/75))
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

- Fix build for packages with `module` type ([#48](https://github.com/esm-dev/esm.sh/issues/48))
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
- Improve NpmPackage resolve (**fix** [#41](https://github.com/esm-dev/esm.sh/issues/41))
- Upgrade esbuild to **0.11.4**
- Upgrade rex to **1.3.0**

# esm-cjs-lexer

A **WASM** module to parse the `module.exports` of a commonjs module for **ESM** converting, powered by [swc](https://github.com/swc-project/swc).

## Installation

[esm-cjs-lexer](https://npmjs.org/esm-cjs-lexer) is a npm package and can be installed with:

```bash
npm i esm-cjs-lexer
```

## Usage

[esm-cjs-lexer](https://npmjs.org/esm-cjs-lexer) provides a `parse` function to parse the `module.exports` of a commonjs module. It returns an object with `exports` and `reexports` fields. For example:

```js
const { parse } = require('esm-cjs-lexer');

// named exports
// exports: ['a', 'b', 'c', '__esModule', 'foo']
const { exports } = parse('index.cjs', `
  exports.a = "a";
  module.exports.b = "b";
  Object.defineProperty(exports, "c", { value: "c" });
  Object.defineProperty(module.exports, "__esModule", { value: true })
  const key = "foo"
  Object.defineProperty(exports, key, { value: "e" });
`);

// reexports
// reexports: ['./lib']
const { reexports } = parse('index.cjs', `
  module.exports = require("./lib");
`);

// object exports(spread supported)
// exports: ['foo', 'baz']
// reexports: ['./lib']
const { exports, reexports } = parse('index.cjs', `
  const foo = 'bar'
  const obj = { baz: 123 }
  module.exports = { foo, ...obj, ...require("./lib") };
`);

// if condition
// exports: ['foo', 'cjs']
const { exports } = parse('index.cjs', `
  module.exports.a = "a";
  if (true) {
    exports.foo = "bar";
  }
  const mtype = "cjs";
  if (mtype === "cjs") {
    exports.cjs = true;
  } else {
    exports.esm = true;
  }
  if (false) {
    exports.ignore = "ignore";
  }
`);

// if condition by checking `process.env.NODE_ENV`
// reexports: ['./index.development']
const { reexports } = parse('index.cjs', `
  if (process.env.NODE_ENV === "development") {
    module.exports = require("./index.development")
  } else {
    module.exports = require("./index.production")
  }
`, { nodeEnv: 'development' });

// block&IIFE
// exports: ['foo', 'baz', '__esModule']
const { exports } = parse('index.cjs', `
  (function () {
    exports.foo = "bar"
    if (true) {
      return
    }
    exports.ignore = '-'
  })();
  {
    exports.baz = 123
  }
  exports.__esModule = true
`);

// function called exports
// exports: ['foo']
const { exports } = parse('index.cjs', `
  function Fn() {
    return { foo: "bar" }
  }
  module.exports = Fn()
`);

// annotated exports
// exports: ['foo', 'bar']
const { exports } = parse('lib.cjs', `
0 && (module.exports = {
 foo,
 bar,
})
`);

// UMD format
// exports: ['foo']
const { exports } = parse('index.cjs', `
  (function (global, factory) {
    typeof exports === 'object' && typeof module !== 'undefined' ? factory(exports) :
    typeof define === 'function' && define.amd ? define(['exports'], factory) :
    (factory((global.MMDParser = global.MMDParser || {})));
  }(this, function (exports) {
    exports.foo = "bar";
  }))
`);

// function reexports
// reexports: ['./lib()']
const { reexports } = parse('index.cjs', `
  module.exports = require("./lib")()
`);

// apply function exports (call mode)
// exports: ['foo']
const { exports } = parse('lib.cjs', `
  module.exports = function() {
    return { foo: 'bar' }
  }
`, { callMode: true });
```

The `parse` function has the following types definition:

```ts
export function parse(
  specifier: string,
  code: string,
  options? {
    nodeEnv?: 'development' | 'production',
    callMode?: boolean,
  }
): {
  exports: string[],
  reexports: string[],
};
```

## Development Setup

You will need [rust](https://www.rust-lang.org/tools/install) 1.56+ and [wasm-pack](https://rustwasm.github.io/wasm-pack/installer/).

## Build

```bash
wasm-pack build --target nodejs
```

## Run tests

```bash
cargo test --all
```

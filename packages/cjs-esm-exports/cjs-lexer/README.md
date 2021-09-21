# cjs-lexer

Parse cjs exports for ESM, powered by [swc](https://github.com/swc-project/swc).

## Development Setup

You will need [rust](https://www.rust-lang.org/tools/install) 1.30+ and [wasm-pack](https://rustwasm.github.io/wasm-pack/installer/).

## Build

```bash
wasm-pack build --target web
```

## Run tests

```bash
cargo test --all
```

# A CLI tool for no-build web apps with esm.sh CDN.

A CLI tool for no-build web apps with esm.sh CDN, written in Go.

## Installation

You can install esm.sh CLI from source code:

```bash
go install github.com/esm-dev/esm.sh
```

You can also install esm.sh CLI via `npm`:

```bash
npm install -g esm.sh
```

Or use `npx` without installation:

```bash
npx esm.sh [command]
```

### Usage

```
$ esm.sh --help
Usage: esm.sh [command] <options>

Commands:
  i, add [...pakcage]   Alias to 'esm.sh im add'.
  im, importmap         Manage "importmap" script.
  init                  Create a new no-build web app with esm.sh CDN.
  serve                 Serve a no-build web app with esm.sh CDN, HMR, transforming TS/Vue/Svelte on the fly.
  build                 Build a no-build web app with esm.sh CDN.
```

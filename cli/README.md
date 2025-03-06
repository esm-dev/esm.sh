# esm.sh CLI

A _nobuild_ tool for modern web development.

## Installation

You can install `esm.sh` CLI from source code:

```bash
go install github.com/esm-dev/esm.sh
```

You can also install `esm.sh` CLI via `npm`:

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
  add, i [...packages]  Alias to 'importmap add'.
  importmap, im         Manage "importmap" script.
  init                  Create a new web app.
  serve                 Serve a web app.
  dev                   Serve a web app in development mode.
```

# esm.sh CLI

A _nobuild_ tool for modern web development.

> [!WARNING]
> The `esm.sh` CLI is still in development and may not be stable. Use it at your own risk.

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
  init                  Create a new web application.
  serve                 Serve a web application.
  dev                   Serve a web app in development mode.
```

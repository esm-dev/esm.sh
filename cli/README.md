# esm.sh CLI

> [!WARNING]
> The `esm.sh` CLI is still in development. Use it at your own risk.

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
  add, i [...packages]    Add packages to the "importmap" script
  update                  Update packages in the "importmap" script
  tidy                    Tidy up the "importmap" script
  init                    Create a new web application
  serve, x                Serve a web application
  dev                     Serve a web application in development mode

Options:
  --help                  Show help message
```

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
  add, i [...packages]    Add specified packages to the "importmap" script in index.html
  update                  Update existing packages in the "importmap" script in index.html
  tidy                    Clean up and optimize the "importmap" script in index.html
  init                    Initialize a new web application
  serve                   Serve the web application in production mode
  dev                     Serve the web application in development mode with live reload

Options:
  --help                  Display this help message
```

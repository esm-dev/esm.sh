# esm.sh CLI

A CLI tool for managing `importmap` script in `index.html`.

## Installation

Install `esm.sh` CLI via curl:

```bash
curl -fsSL https://esm.sh/install | bash
```

To install `esm.sh` CLI from source code, you need to have [Go](https://go.dev/dl) installed.

```bash
go install github.com/esm-dev/esm.sh
```

Or install `esm.sh` CLI via `npm`:

```bash
npm install -g esm.sh
```

Or use `npx esm.sh` without installation:

```bash
npx esm.sh [command]
```

### Usage

```
$ esm.sh --help
Usage: esm.sh [command] [options]

Commands:
  add [...imports]      Add imports to the "importmap" script in index.html
  tidy                  Clean up and optimize the "importmap" script in index.html

Options:
  --version, -v         Show the version
  --help, -h            Display this help message
```

Download modules into `vendor/` beside `index.html` with `--download` or `-D`:

```bash
esm.sh add -D react@19
```

The import map then points to local files, such as `./vendor/react_19.3.0/es2022/react.mjs`.
The CLI downloads CDN imports and their dependencies, rewrites static imports and literal dynamic
imports to local paths, and generates integrity hashes for the downloaded files. Use `--no-sri`
to omit the hashes. Without `--download`, imports continue to use CDN URLs.

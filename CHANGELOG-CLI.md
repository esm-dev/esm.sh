# CLI Changelog

## v0.1.0

Introduce esm.sh CLI, a import maps manager for modern web development written in golang. Features include:

- Add imports from esm.sh CDN
- Tidy import map

Usage:

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

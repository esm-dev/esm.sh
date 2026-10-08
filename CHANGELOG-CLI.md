# CLI Changelog

## v0.2.0

- Add `esm.sh add --download` (`-D`) to download modules and dependencies into `vendor/`. Rewrite imports to local paths and generate integrity hashes.
- Preserve HTML and import-map configuration during import updates. Keep files unchanged if import resolution or downloads fail.
- Preserve custom imports, scopes, and development builds with `esm.sh tidy`.
- Fix dependency scopes, concurrent import updates, and JSON escapes in import maps.
- Fix GitHub and external scoped package URLs.
- Return a nonzero exit status when `add` or `tidy` fails.
- Fix npm binary downloads, platform names, and install paths.

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

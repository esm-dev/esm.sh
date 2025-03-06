# esm.sh/web

A golang `http.Handler` that serves _nobuild_ web applications.

- Web applications are served _as-is_ without any build step.
- Transpiles TypeScript, JSX, Vue, Svelte on-the-fly.
- Built-in UnoCSS generator.
- HMR (Hot Module Replacement).

## Installation

```sh
go get -u github.com/esm-dev/esm.sh
```

## Usage

```go
package main

import (
  "net/http"
  "log"

  "github.com/esm-dev/esm.sh/web"
)

func main() {
  http.Handle("GET /", web.New(web.Config{
    Dev: false,
    RootDir: "/path/to/webapp",
  }))
  log.Fatal(http.ListenAndServe(":8080", nil))
}
```

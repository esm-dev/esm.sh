package main

import (
	"embed"

	"github.com/esm-dev/esm.sh/server"
)

//go:embed build.ts
//go:embed run.ts
//go:embed uno.ts
//go:embed server/embed
//go:embed README.md
var fs embed.FS

func main() {
	server.Serve(&fs)
}

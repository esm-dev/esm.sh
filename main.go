package main

import (
	"embed"

	"github.com/esm-dev/esm.sh/server"
)

//go:embed build.ts
//go:embed hot
//go:embed hot.ts
//go:embed README.md
//go:embed run.ts
//go:embed server/embed
var fs embed.FS

func main() {
	server.Serve(&fs)
}

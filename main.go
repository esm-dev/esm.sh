package main

import (
	"embed"

	"github.com/esm-dev/esm.sh/server"
)

//go:embed README.md
//go:embed CLI.ts
//go:embed server/embed
var fs embed.FS

func main() {
	server.Serve(&fs)
}

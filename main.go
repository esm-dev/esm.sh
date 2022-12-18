package main

import (
	"embed"

	"github.com/ije/esm.sh/server"
)

//go:embed README.md
//go:embed server/embed
var fs embed.FS

func main() {
	server.Serve(&fs)
}

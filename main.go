package main

import (
	"embed"

	"esm.sh/server"
)

//go:embed README.md
//go:embed server/embed
var fs embed.FS

func main() {
	server.Serve(&fs)
}

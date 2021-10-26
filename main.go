package main

import (
	"embed"

	"esm.sh/server"
)

//go:embed README.md
//go:embed test/browser
var fs embed.FS

func main() {
	server.Serve(&fs)
}

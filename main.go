package main

import (
	"embed"

	"esm.sh/server"
)

//go:embed embed
//go:embed README.md
var fs embed.FS

func main() {
	server.Serve(&fs)
}

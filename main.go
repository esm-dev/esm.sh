package main

import (
	"embed"

	"esm.sh/server"
)

//go:embed README.md assets/index.html polyfills/*.js types/*.d.ts
var fs embed.FS

func main() {
	server.Serve(&fs)
}

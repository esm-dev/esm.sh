package main

import (
	"embed"

	"esm.sh/server"
)

//go:embed README.md
//go:embed assets
//go:embed polyfills/*.js
//go:embed types/*.d.ts
var fs embed.FS

func main() {
	server.Serve(&fs)
}

package main

import (
	"embed"

	"github.com/esm-dev/esm.sh/server"
)

//go:embed README.md
//go:embed CLI.deno.ts
//go:embed CLI.node.js
//go:embed build.ts
//go:embed server.deno.ts
//go:embed server.node.js
//go:embed server/embed
var fs embed.FS

func main() {
	server.Serve(&fs)
}

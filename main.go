package main

import (
	"embed"

	"github.com/esm-dev/esmd"
)

//go:embed README.md
//go:embed server/embed
//go:embed test/browser
var frontendEmbedFS embed.FS

func main() {
	esmd.New(esmd.WithFrontendEmbedFS(frontendEmbedFS))
}

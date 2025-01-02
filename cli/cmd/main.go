package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/esm-dev/esm.sh/cli"
)

const helpMessage = "\033[30mesm.sh - A no-build CDN for modern web development.\033[0m" + `

Usage: esm.sh [command] [options]

Commands:
  add    Add dependencies to the "importmap" script
  init   Create a new esm.sh web app
  serve  Serve an esm.sh web app
`

//go:embed internal
//go:embed demo
var efs embed.FS

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "add":
			cli.Add()
		case "init":
			cli.Init(&efs)
		case "serve":
			cli.Serve(&efs)
		default:
			fmt.Print(helpMessage)
		}
	} else {
		fmt.Print(helpMessage)
	}
}

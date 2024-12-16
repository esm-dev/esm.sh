package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/esm-dev/esm.sh/cli"
)

const helpMessage = "\033[30mesm.sh - The no-build CDN for modern web development.\033[0m" + `

Usage: esm.sh [command] [options]

Commands:
  add   Add NPM packages to the "importmap" script
  init  Create a new esm.sh web app
  run   Serve an esm.sh web app
`

//go:embed internal
//go:embed demo
var efs embed.FS

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init":
			cli.Init(&efs)
		case "add":
			cli.Add()
		case "run":
			cli.Run(&efs)
		default:
			fmt.Print(helpMessage)
		}
	} else {
		fmt.Print(helpMessage)
	}
}

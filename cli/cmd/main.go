package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/esm-dev/esm.sh/cli"
)

const helpMessage = "\033[90mesm.sh - The no-build CDN for modern web development.\033[0m" + `

Usage: esm.sh [command] [options]

Commands:
  init  Create a new esm.sh web app
  run   Serve an esm.sh web app
  add   Add NPM packages to the "importmap" script
`

//go:embed internal
//go:embed demo
var efs embed.FS

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init":
			cli.Init(&efs)
		case "run":
			cli.Run(&efs)
		case "add":
			if len(os.Args) > 2 {
				cli.Add(os.Args[2:])
			}
		default:
			fmt.Print(helpMessage)
		}
	} else {
		fmt.Print(helpMessage)
	}
}

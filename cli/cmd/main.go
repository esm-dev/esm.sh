package main

import (
	"embed"
	"flag"
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

//go:embed assets
//go:embed demo
var assets embed.FS

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init":
			cli.Init(&assets)
		case "run":
			port := flag.Int("port", 3000, "port to serve on")
			flag.Parse()
			cli.Serve(&assets, flag.Arg(1), *port)
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

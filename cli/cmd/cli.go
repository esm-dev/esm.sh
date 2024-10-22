package main

import (
	"embed"
	"flag"
	"fmt"
	"os"

	"github.com/esm-dev/esm.sh/cli"
)

const helpMessage = `Usage: esm.sh [command] [options]

Commands:
  run  Serve the current directory
  add  Add NPM packages to the 'importmap' script of the root index.html
`

//go:embed assets
var assets embed.FS

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
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

package main

import (
	"fmt"
	"os"

	"github.com/esm-dev/esm.sh/cli"
)

const helpMessage = "\033[30mesm.sh - A nobuild tool for modern web development.\033[0m" + `

Usage: esm.sh [command] <options>

Commands:
  add, i [...packages]    Add packages to the "importmap" script
  update                  Update packages in the "importmap" script
  tidy                    Tidy up the "importmap" script
  init                    Create a new web application
  serve, x                Serve a web application
  dev                     Serve a web application in development mode

Options:
  --help                  Show help message
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(helpMessage)
		return
	}
	switch command := os.Args[1]; command {
	case "add", "i":
		cli.Add()
	case "update":
		cli.Update()
	case "tidy":
		cli.Tidy()
	case "init":
		cli.Init()
	case "serve", "x":
		cli.Serve(false)
	case "dev":
		cli.Serve(true)
	default:
		fmt.Print(helpMessage)
	}
}

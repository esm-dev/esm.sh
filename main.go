package main

import (
	"fmt"
	"os"

	"github.com/esm-dev/esm.sh/cli"
)

const helpMessage = "\033[30mesm.sh - A nobuild tool for modern web development.\033[0m" + `

Usage: esm.sh [command] <options>

Commands:
  add, i [...packages]  Alias to 'importmap add'.
  importmap, im         Manage "importmap" script.
  init                  Create a new web app.
  serve                 Serve a web app.
  dev                   Serve a web app in development mode.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(helpMessage)
		return
	}
	switch command := os.Args[1]; command {
	case "add", "i":
		cli.ManageImportMap("add")
	case "importmap", "im":
		cli.ManageImportMap("")
	case "init":
		cli.Init()
	case "serve":
		cli.Serve()
	case "dev":
		cli.Dev()
	default:
		fmt.Print(helpMessage)
	}
}

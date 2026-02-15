package cli

import (
	"fmt"
	"os"
)

const helpMessage = "\033[30mesm.sh - A no-build tool for modern web development.\033[0m" + `

Usage: esm.sh [command] [options]

Commands:
  add [...packages]     Add specified packages to the "importmap" in index.html
  tidy                  Clean up and optimize the "importmap" in index.html
  init                  Initialize a new no-build web app
  serve                 Serve the web app in "production" mode
  dev                   Serve the web app in "development" mode with live reload

Options:
  --version, -v         Show the version
  --help, -h            Display this help message
`

func Run() {
	if len(os.Args) < 2 {
		fmt.Print(helpMessage)
		return
	}
	switch command := os.Args[1]; command {
	case "add":
		Add()
	case "tidy":
		Tidy()
	case "init":
		Init()
	case "serve":
		Serve()
	case "dev":
		Dev()
	case "version":
		fmt.Println("esm.sh CLI " + VERSION)
	default:
		for _, arg := range os.Args[1:] {
			if arg == "--version" {
				fmt.Println("esm.sh CLI " + VERSION)
				return
			}
			if arg == "-v" {
				fmt.Println(VERSION)
				return
			}
		}
		fmt.Print(helpMessage)
	}
}

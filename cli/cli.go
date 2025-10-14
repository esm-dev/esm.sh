package cli

import (
	"fmt"
	"os"
)

const helpMessage = "\033[30mesm.sh - A no-build tool for modern web development.\033[0m" + `

Usage: esm.sh [command] [options]

Commands:
  add, i [...packages]    Add specified packages to the "importmap" script in index.html
  update                  Update existing packages in the "importmap" script in index.html
  tidy                    Clean up and optimize the "importmap" script in index.html
  init                    Initialize a new web application
  serve                   Serve the web application in production mode
  dev                     Serve the web application in development mode with live reload

Options:
  --version, -v           Show the version of esm.sh CLI
  --help, -h              Display this help message
`

func Run() {
	if len(os.Args) < 2 {
		fmt.Print(helpMessage)
		return
	}
	switch command := os.Args[1]; command {
	case "add", "i":
		Add()
	case "update":
		Update()
	case "tidy":
		Tidy()
	case "init":
		Init()
	case "serve":
		Serve(false)
	case "dev":
		Serve(true)
	case "version":
		fmt.Println("esm.sh CLI " + Version)
	default:
		for _, arg := range os.Args[1:] {
			if arg == "-v" {
				fmt.Println(Version)
				return
			}
			if arg == "--version" {
				fmt.Println("esm.sh CLI " + Version)
				return
			}
		}
		fmt.Print(helpMessage)
	}
}

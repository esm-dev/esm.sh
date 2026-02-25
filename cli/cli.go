package cli

import (
	"fmt"
	"os"
)

const helpMessage = "\033[30mesm.sh - A no-build tool for modern web development.\033[0m" + `

Usage: esm.sh [command] [options]

Commands:
  add [...imports]      Add imports to the "importmap" script in index.html
  tidy                  Clean up and optimize the "importmap" script in index.html

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

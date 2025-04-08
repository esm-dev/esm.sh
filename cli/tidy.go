package cli

import (
	"flag"
	"fmt"
)

const tidyHelpMessage = "\033[30mesm.sh - A nobuild tool for modern web development.\033[0m" + `

Usage: esm.sh tidy [options]

Options:
  --help       Show help message
`

// Tidy tidies up "importmap" script
func Tidy() {
	help := flag.Bool("help", false, "Show help message")
	flag.Parse()

	if *help {
		fmt.Print(tidyHelpMessage)
		return
	}

	fmt.Println("Tidying up...")
}

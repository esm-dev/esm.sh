package cli

import (
	"flag"
	"fmt"
)

const updateHelpMessage = "\033[30mesm.sh - A nobuild tool for modern web development.\033[0m" + `

Usage: esm.sh update [...packages] [options]

Examples:
  esm.sh update        # update all packages in the "importmap" script
	esm.sh update react  # update a specific package

Arguments:
  [...packages]        Packages to update, separated by space

Options:
  --help               Show help message
`

// Update updates packages in "importmap" script
func Update() {
	help := flag.Bool("help", false, "Show help message")
	arg0, argMore := parseCommandFlag(2)

	if *help {
		fmt.Print(updateHelpMessage)
		return
	}

	var packages []string
	if arg0 != "" {
		packages = append(packages, arg0)
		packages = append(packages, argMore...)
	}

	fmt.Println("Updating packages:", packages)
}

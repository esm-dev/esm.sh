package cli

import (
	"fmt"
	"os"
)

// Add NPM packages to the "importmap" script
func Add() {
	packages := os.Args[2:]
	if len(packages) == 0 {
		return
	}
	fmt.Println(packages)
}

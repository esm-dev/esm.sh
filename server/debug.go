//go:build debug

package server

import (
	"os"
	"path"
)

// debug mode
const DEBUG = true

// always "DEV" in DEBUG mode
const VERSION = "DEV"

// mock embed.FS reads files from the current working directory in DEBUG mode
var embedFS MockEmbedFS

type MockEmbedFS struct{}

func (fs MockEmbedFS) ReadFile(name string) ([]byte, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path.Join(cwd, "server", name))
}

package server

import (
	"os"
	"strings"
)

// MakePathOsAgnostic makes the given path OS agnostic by replacing backslashes with forward slashes.
func MakePathOsAgnostic(path string) string {
	if os.PathSeparator == '\\' {
		return strings.ReplaceAll(path, "\\", "/")
	}

	return path
}

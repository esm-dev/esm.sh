package web

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
)

var (
	moduleExts = []string{".js", ".mjs", ".jsx", ".ts", ".mts", ".tsx", ".svelte", ".vue"}
)

// isHttpSepcifier returns true if the specifier is a remote URL.
func isHttpSepcifier(specifier string) bool {
	return strings.HasPrefix(specifier, "https://") || strings.HasPrefix(specifier, "http://")
}

// isRelPathSpecifier returns true if the specifier is a local path.
func isRelPathSpecifier(specifier string) bool {
	return strings.HasPrefix(specifier, "./") || strings.HasPrefix(specifier, "../")
}

// isAbsPathSpecifier returns true if the specifier is an absolute path.
func isAbsPathSpecifier(specifier string) bool {
	return strings.HasPrefix(specifier, "/") || strings.HasPrefix(specifier, "file://")
}

// endsWith returns true if the given string ends with any of the suffixes.
func endsWith(s string, suffixs ...string) bool {
	for _, suffix := range suffixs {
		if strings.HasSuffix(s, suffix) {
			return true
		}
	}
	return false
}

// run executes the given command and returns the output.
func run(cmd string, args ...string) (output []byte, err error) {
	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	c := exec.Command(cmd, args...)
	c.Dir = os.TempDir()
	c.Stdout = &outBuf
	c.Stderr = &errBuf
	err = c.Run()
	if err != nil {
		if errBuf.Len() > 0 {
			err = errors.New(errBuf.String())
		}
		return
	}
	output = outBuf.Bytes()
	return
}

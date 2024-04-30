package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
)

const EOL = "\n"

var (
	regexpFullVersion     = regexp.MustCompile(`^\d+\.\d+\.\d+[\w\.\+\-]*$`)
	regexpFullVersionPath = regexp.MustCompile(`(\w)@(v?\d+\.\d+\.\d+[\w\.\+\-]*|[0-9a-f]{7})(/|$)`)
	regexpLocPath         = regexp.MustCompile(`(\.m?js):\d+:\d+$`)
	regexpJSIdent         = regexp.MustCompile(`^[a-zA-Z_$][\w$]*$`)
	regexpGlobalIdent     = regexp.MustCompile(`__[a-zA-Z]+\$`)
	regexpVarEqual        = regexp.MustCompile(`var ([a-zA-Z]+)\s*=\s*[a-zA-Z]+$`)
)

var (
	esExts = []string{".mjs", ".js", ".jsx", ".mts", ".ts", ".tsx", ".cjs"}
)

// isHttpSepcifier returns true if the import path is a remote URL.
func isHttpSepcifier(importPath string) bool {
	return strings.HasPrefix(importPath, "https://") || strings.HasPrefix(importPath, "http://")
}

// isRelativeSpecifier returns true if the import path is a local path.
func isRelativeSpecifier(importPath string) bool {
	return strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../") || importPath == "." || importPath == ".."
}

// semverLessThan returns true if the version a is less than the version b.
func semverLessThan(a string, b string) bool {
	return semver.MustParse(a).LessThan(semver.MustParse(b))
}

// includes returns true if the given string is included in the given array.
func includes(a []string, s string) bool {
	if len(a) == 0 {
		return false
	}
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}

// filter returns a new array with all elements that pass the test implemented by the provided function.
func filter(a []string, fn func(s string) bool) []string {
	l := len(a)
	if l == 0 {
		return nil
	}
	b := make([]string, l)
	i := 0
	for _, v := range a {
		if fn(v) {
			b[i] = v
			i++
		}
	}
	return b[:i]
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

// stripModuleExt strips the module extension from the given string.
func stripModuleExt(s string) string {
	for _, ext := range esExts {
		if strings.HasSuffix(s, ext) {
			return s[:len(s)-len(ext)]
		}
	}
	return s
}

// existsDir returns true if the given path is a directory.
func existsDir(filepath string) bool {
	fi, err := os.Lstat(filepath)
	return err == nil && fi.IsDir()
}

// existsFile returns true if the given path is a file.
func existsFile(filepath string) bool {
	fi, err := os.Lstat(filepath)
	return err == nil && !fi.IsDir()
}

// ensureDir creates a directory if it does not exist.
func ensureDir(dir string) (err error) {
	_, err = os.Lstat(dir)
	if err != nil && os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0755)
	}
	return
}

// findFiles returns a list of files in the given directory.
func findFiles(root string, dir string, fn func(p string) bool) ([]string, error) {
	rootDir, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		name := entry.Name()
		path := name
		if dir != "" {
			path = dir + "/" + name
		}
		if entry.IsDir() {
			if name == "node_modules" {
				continue
			}
			subFiles, err := findFiles(filepath.Join(rootDir, name), path, fn)
			if err != nil {
				return nil, err
			}
			n := len(files)
			files = make([]string, n+len(subFiles))
			for i, f := range subFiles {
				files[i+n] = f
			}
			copy(files, subFiles)
		} else {
			if fn(path) {
				files = append(files, path)
			}
		}
	}
	return files, nil
}

// btoaUrl converts a string to a base64 string.
func btoaUrl(s string) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString([]byte(s)), "=")
}

// atobUrl converts a base64 string to a string.
func atobUrl(s string) (string, error) {
	if l := len(s) % 4; l > 0 {
		s += strings.Repeat("=", 4-l)
	}
	data, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// removeHttpPrefix removes the http/https prefix from the given string.
func removeHttpPrefix(s string) (string, error) {
	if strings.HasPrefix(s, "http://") {
		return s[7:], nil
	} else if strings.HasPrefix(s, "https://") {
		return s[8:], nil
	} else {
		return "", fmt.Errorf("not a http/https url: %s", s)
	}
}

// concatBytes concatenates two byte slices.
func concatBytes(a, b []byte) []byte {
	c := make([]byte, len(a)+len(b))
	copy(c, a)
	copy(c[len(a):], b)
	return c
}

// mustEncodeJSON encodes the given value to a JSON byte slice.
func mustEncodeJSON(v interface{}) []byte {
	buf := bytes.NewBuffer(nil)
	err := json.NewEncoder(buf).Encode(v)
	if err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// parseJSONFile parses the given JSON file and stores the result in the value pointed to by v.
func parseJSONFile(filename string, v interface{}) (err error) {
	var file *os.File
	file, err = os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()
	return json.NewDecoder(file).Decode(v)
}

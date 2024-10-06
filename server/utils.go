package server

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
)

const EOL = "\n"

var (
	regexpVersion       = regexp.MustCompile(`^[\w\.\+\-]+$`)
	regexpVersionStrict = regexp.MustCompile(`^\d+\.\d+\.\d+[\w\.\+\-]*$`)
	regexpVuePath       = regexp.MustCompile(`/\*?vue@([\w\.\+\-]+)($|/)`)
	regexpSveltePath    = regexp.MustCompile(`/\*?svelte@([\w\.\+\-]+)($|/)`)
	regexpLocPath       = regexp.MustCompile(`:\d+:\d+$`)
	regexpJSIdent       = regexp.MustCompile(`^[a-zA-Z_$][\w$]*$`)
	regexpGlobalIdent   = regexp.MustCompile(`__[a-zA-Z]+\$`)
	regexpVarEqual      = regexp.MustCompile(`var ([\w$]+)\s*=\s*[\w$]+$`)
	regexpDomain        = regexp.MustCompile(`^[a-z0-9\-]+(\.[a-z0-9\-]+)*\.[a-z]+$`)
)

// isHttpSepcifier returns true if the specifier is a remote URL.
func isHttpSepcifier(specifier string) bool {
	return strings.HasPrefix(specifier, "https://") || strings.HasPrefix(specifier, "http://")
}

// isRelativeSpecifier returns true if the specifier is a local path.
func isRelativeSpecifier(specifier string) bool {
	return strings.HasPrefix(specifier, "./") || strings.HasPrefix(specifier, "../") || specifier == "." || specifier == ".."
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

// endsWith returns true if the given string ends with any of the suffixes.
func endsWith(s string, suffixs ...string) bool {
	for _, suffix := range suffixs {
		if strings.HasSuffix(s, suffix) {
			return true
		}
	}
	return false
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

// relPath returns a relative path from the base path to the target path.
func relPath(basePath, targetPath string) (string, error) {
	rp, err := filepath.Rel(basePath, targetPath)
	if err == nil && !isRelativeSpecifier(rp) {
		rp = "./" + rp
	}
	return rp, err
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
			newFiles := make([]string, len(files)+len(subFiles))
			copy(newFiles, files)
			copy(newFiles[len(files):], subFiles)
			files = newFiles
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

// removeHttpUrlProtocol removes the `http[s]:` protocol from the given url.
func removeHttpUrlProtocol(url string) string {
	if strings.HasPrefix(url, "https://") {
		return url[6:]
	}
	if strings.HasPrefix(url, "http://") {
		return url[5:]
	}
	return url
}

// appendVaryHeader appends the given key to the `Vary` header.
func appendVaryHeader(header http.Header, key string) {
	vary := header.Get("Vary")
	if vary == "" {
		header.Set("Vary", key)
	} else {
		header.Set("Vary", vary+", "+key)
	}
}

// toEnvName converts the given string to an environment variable name.
func toEnvName(s string) string {
	runes := []rune(s)
	for i, r := range runes {
		if (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') {
			runes[i] = r
		} else if r >= 'a' && r <= 'z' {
			runes[i] = r - 'a' + 'A'
		} else {
			runes[i] = '_'
		}
	}
	return string(runes)
}

// concatBytes concatenates two byte slices.
func concatBytes(a, b []byte) []byte {
	c := make([]byte, len(a)+len(b))
	copy(c, a)
	copy(c[len(a):], b)
	return c
}

// pathJoin joins the given path segments with `/`.
func pathJoin(path ...string) string {
	a := make([]string, len(path))
	j := 0
	for _, p := range path {
		if p != "" && p != "." {
			a[j] = p
			j++
		}
	}
	return strings.Join(a[:j], "/")
}

// run executes the given command and returns the output.
func run(cmd string, args ...string) (output []byte, err error) {
	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	c := exec.Command(cmd, args...)
	c.Stdout = &outBuf
	c.Stderr = &errBuf
	err = c.Run()
	if err != nil {
		if errBuf.Len() > 0 {
			err = fmt.Errorf("%s: %s", err, errBuf.String())
		}
		return
	}
	if errBuf.Len() > 0 {
		err = fmt.Errorf("%s", errBuf.String())
		return
	}
	output = outBuf.Bytes()
	return
}

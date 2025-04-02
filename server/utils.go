package server

import (
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/ije/gox/valid"
)

// isJsReservedWord returns true if the given string is a reserved word in JavaScript.
func isJsReservedWord(word string) bool {
	switch word {
	case "abstract", "arguments", "await", "boolean", "break", "byte", "case", "catch", "char", "class", "const", "continue", "debugger", "default", "delete", "do", "double", "else", "enum", "eval", "export", "extends", "false", "final", "finally", "float", "for", "function", "goto", "if", "implements", "import", "in", "instanceof", "int", "interface", "let", "long", "native", "new", "null", "package", "private", "protected", "public", "return", "short", "static", "super", "switch", "synchronized", "this", "throw", "throws", "transient", "true", "try", "typeof", "var", "void", "volatile", "while", "with", "yield":
		return true
	}
	return false
}

// isJsIdentifier returns true if the given string is a valid JavaScript identifier.
func isJsIdentifier(s string) bool {
	if len(s) == 0 {
		return false
	}
	leadingChar := s[0]
	if !((leadingChar >= 'a' && leadingChar <= 'z') || (leadingChar >= 'A' && leadingChar <= 'Z') || leadingChar == '_' || leadingChar == '$') {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '$') {
			return false
		}
	}
	return true
}

// isCommitish returns true if the given string is a commit hash.
func isCommitish(s string) bool {
	return len(s) >= 7 && len(s) <= 40 && valid.IsHexString(s)
}

// isNodeBuiltinSpecifier checks if the given specifier is a node.js built-in module.
func isNodeBuiltinSpecifier(specifier string) bool {
	return strings.HasPrefix(specifier, "node:") && nodeBuiltinModules[specifier[5:]]
}

// isJsonModuleSpecifier returns true if the specifier is a json module.
func isJsonModuleSpecifier(specifier string) bool {
	if !strings.HasSuffix(specifier, ".json") {
		return false
	}
	_, _, subpath, _ := splitEsmPath(specifier)
	return subpath != "" && strings.HasSuffix(subpath, ".json")
}

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

// checks if the given hostname is a local address.
func isLocalhost(hostname string) bool {
	return hostname == "localhost" || hostname == "127.0.0.1" || (valid.IsIPv4(hostname) && strings.HasPrefix(hostname, "192.168."))
}

// semverLessThan returns true if the version a is less than the version b.
func semverLessThan(a string, b string) bool {
	va, err1 := semver.NewVersion(a)
	if err1 != nil {
		return false
	}
	vb, err2 := semver.NewVersion(b)
	return err2 == nil && va.LessThan(vb)
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
	if err == nil && !isRelPathSpecifier(rp) {
		rp = "./" + rp
	}
	return rp, err
}

// findFiles returns a list of files in the given directory.
func findFiles(root string, dir string, filter func(filename string) bool) ([]string, error) {
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
		filename := name
		if dir != "" {
			filename = dir + "/" + name
		}
		if entry.IsDir() {
			if name == "node_modules" {
				continue
			}
			subFiles, err := findFiles(filepath.Join(rootDir, name), filename, filter)
			if err != nil {
				return nil, err
			}
			newFiles := make([]string, len(files)+len(subFiles))
			copy(newFiles, files)
			copy(newFiles[len(files):], subFiles)
			files = newFiles
		} else {
			if filter(filename) {
				files = append(files, filename)
			}
		}
	}
	return files, nil
}

// btoaUrl converts a string to a base64 string.
func btoaUrl(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

// atobUrl converts a base64 string to a string.
func atobUrl(s string) (string, error) {
	data, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(data), nil
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

// concatBytes concatenates two byte slices.
func concatBytes(a, b []byte) []byte {
	al, bl := len(a), len(b)
	if al == 0 {
		return b[0:]
	}
	if bl == 0 {
		return a[0:]
	}
	c := make([]byte, al+bl)
	copy(c, a)
	copy(c[al:], b)
	return c
}

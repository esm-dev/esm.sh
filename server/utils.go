package server

import (
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ije/esbuild-internal/js_ast"
	"github.com/ije/esbuild-internal/js_parser"
	"github.com/ije/esbuild-internal/logger"
)

var (
	regexpFullVersion      = regexp.MustCompile(`^\d+\.\d+\.\d+[\w\.\+\-]*$`)
	regexpFullVersionPath  = regexp.MustCompile(`([^/])@\d+\.\d+\.\d+[\w\.\+\-]*(/|$)`)
	regexpBuildVersionPath = regexp.MustCompile(`^/v\d+(/|$)`)
	regexpLocPath          = regexp.MustCompile(`(\.js):\d+:\d+$`)
	regexpJSIdent          = regexp.MustCompile(`^[a-zA-Z_$][\w$]*$`)
	regexpGlobalIdent      = regexp.MustCompile(`__[a-zA-Z]+\$`)
)

var httpClient = &http.Client{
	Transport: &http.Transport{
		Dial: func(network, addr string) (conn net.Conn, err error) {
			conn, err = net.DialTimeout(network, addr, 15*time.Second)
			if err != nil {
				return conn, err
			}

			// Set a one-time deadline for potential SSL handshaking
			conn.SetDeadline(time.Now().Add(60 * time.Second))
			return conn, nil
		},
		MaxIdleConnsPerHost:   6,
		ResponseHeaderTimeout: 60 * time.Second,
		Proxy:                 http.ProxyFromEnvironment,
	},
}

func splitPkgPath(pathname string) (pkgName string, submodule string) {
	a := strings.Split(strings.Trim(pathname, "/"), "/")
	pkgName = a[0]
	submodule = strings.Join(a[1:], "/")
	if strings.HasPrefix(pkgName, "@") && len(a) > 1 {
		pkgName = a[0] + "/" + a[1]
		submodule = strings.Join(a[2:], "/")
	}
	return
}

// isRemoteImport returns true if the import path is a remote URL.
func isRemoteImport(importPath string) bool {
	return strings.HasPrefix(importPath, "https://") || strings.HasPrefix(importPath, "http://")
}

// isLocalImport returns true if the import path is a local path.
func isLocalImport(importPath string) bool {
	return strings.HasPrefix(importPath, "file://") || strings.HasPrefix(importPath, "/") || strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../") || importPath == "." || importPath == ".."
}

// include returns true if the given string is included in the given array.
func includes(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}

func startsWith(s string, prefixs ...string) bool {
	for _, prefix := range prefixs {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

func endsWith(s string, suffixs ...string) bool {
	for _, suffix := range suffixs {
		if strings.HasSuffix(s, suffix) {
			return true
		}
	}
	return false
}

func dirExists(filepath string) bool {
	fi, err := os.Lstat(filepath)
	return err == nil && fi.IsDir()
}

func fileExists(filepath string) bool {
	fi, err := os.Lstat(filepath)
	return err == nil && !fi.IsDir()
}

func ensureDir(dir string) (err error) {
	_, err = os.Stat(dir)
	if err != nil && os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0755)
	}
	return
}

func readDir(root string) ([]string, error) {
	rootDir, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	var files []string
	err = filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			name := strings.TrimPrefix(path, rootDir+"/")
			if !strings.HasPrefix(name, "node_modules/") && !strings.HasPrefix(name, ".") {
				files = append(files, name)
			}
		}
		return nil
	})
	return files, err
}

func clearDir(dir string) (err error) {
	os.RemoveAll(dir)
	err = os.MkdirAll(dir, 0755)
	return
}

func btoaUrl(s string) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString([]byte(s)), "=")
}

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

func kill(pidFile string) (err error) {
	if pidFile == "" {
		return
	}
	data, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			err = nil
		}
		return
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	return process.Kill()
}

func validateJS(filename string) (isESM bool, hasDefaultExport bool, err error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return
	}
	log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
	ast, pass := js_parser.Parse(log, logger.Source{
		Index:          0,
		KeyPath:        logger.Path{Text: "<stdin>"},
		PrettyPath:     "<stdin>",
		Contents:       string(data),
		IdentifierName: "stdin",
	}, js_parser.Options{})
	if !pass {
		err = errors.New("invalid syntax, require javascript/typescript")
		return
	}
	isESM = ast.ExportsKind == js_ast.ExportsESM
	_, hasDefaultExport = ast.NamedExports["default"]
	return
}

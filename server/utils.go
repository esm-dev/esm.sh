package server

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/ije/esbuild-internal/js_ast"
	"github.com/ije/esbuild-internal/js_parser"
	"github.com/ije/esbuild-internal/logger"
)

var (
	regexpFullVersion      = regexp.MustCompile(`^\d+\.\d+\.\d+[\w\.\+\-]*$`)
	regexpFullVersionPath  = regexp.MustCompile(`(\w)@(v?\d+\.\d+\.\d+[\w\.\+\-]*|[0-9a-f]{10})(/|$)`)
	regexpBuildVersionPath = regexp.MustCompile(`^/v\d+(/|$)`)
	regexpLocPath          = regexp.MustCompile(`(\.js):\d+:\d+$`)
	regexpJSIdent          = regexp.MustCompile(`^[a-zA-Z_$][\w$]*$`)
	regexpGlobalIdent      = regexp.MustCompile(`__[a-zA-Z]+\$`)
)

var httpClient = &http.Client{
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: transportDialContext(&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}),
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
}

func transportDialContext(dialer *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	return dialer.DialContext
}

func fetch(url string) (res *http.Response, err error) {
	return httpClient.Get(url)
}

// isRemoteSpecifier returns true if the import path is a remote URL.
func isRemoteSpecifier(importPath string) bool {
	return strings.HasPrefix(importPath, "https://") || strings.HasPrefix(importPath, "http://")
}

// isLocalSpecifier returns true if the import path is a local path.
func isLocalSpecifier(importPath string) bool {
	return strings.HasPrefix(importPath, "file://") || strings.HasPrefix(importPath, "/") || strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../") || importPath == "." || importPath == ".."
}

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
	_, err = os.Lstat(dir)
	if err != nil && os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0755)
	}
	return
}

func findFiles(root string, fn func(p string) bool) ([]string, error) {
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
		if entry.IsDir() {
			if name == "node_modules" {
				continue
			}
			subFiles, err := findFiles(filepath.Join(rootDir, name), fn)
			if err != nil {
				return nil, err
			}
			n := len(files)
			files = make([]string, n+len(subFiles))
			for i, f := range subFiles {
				files[i+n] = filepath.Join(name, f)
			}
			copy(files, subFiles)
		} else {
			if fn(name) {
				files = append(files, name)
			}
		}
	}
	return files, nil
}

func readDirnames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	n := 0
	for _, entry := range entries {
		if entry.IsDir() {
			names[n] = entry.Name()
			n++
		}
	}
	return names[:n], nil
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
		return nil
	}
	data, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			err = nil
		}
		return
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil || pid <= 0 {
		return nil
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	return process.Kill()
}

func isJSIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '$'
}

func validateJS(filename string) (isESM bool, namedExports []string, err error) {
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
	namedExports = make([]string, len(ast.NamedExports))
	i := 0
	for name := range ast.NamedExports {
		namedExports[i] = name
		i++
	}
	return
}

var purgeDelay = 24 * time.Hour

func toPurge(pkg string, destDir string) {
	timer := time.AfterFunc(purgeDelay, func() {
		purgeTimers.Delete(pkg)
		lock := getInstallLock(pkg)
		lock.Lock()
		log.Debugf("Purging %s...", pkg)
		os.RemoveAll(destDir)
		lock.Unlock()
	})
	purgeTimers.Store(pkg, timer)
}

func restorePurgeTimers(npmDir string) {
	dirnames, err := readDirnames(npmDir)
	if err != nil {
		return
	}
	var pkgs []string
	for _, name := range dirnames {
		if name == "gh" {
			owners, err := readDirnames(path.Join(npmDir, name))
			if err == nil {
				for _, owner := range owners {
					repos, err := readDirnames(path.Join(npmDir, "gh", owner))
					if err != nil {
						return
					}
					for _, repo := range repos {
						pkgs = append(pkgs, "gh/"+owner+"/"+repo)
					}
				}
			}
		} else if strings.HasPrefix(name, "@") {
			subdirnames, err := readDirnames(path.Join(npmDir, name))
			if err == nil {
				for _, subdirname := range subdirnames {
					pkgs = append(pkgs, name+"/"+subdirname)
				}
			}
		} else {
			pkgs = append(pkgs, name)
		}
	}
	for _, pkg := range pkgs {
		toPurge(pkg, path.Join(npmDir, pkg))
	}
	log.Debugf("Restored %d purge timers", len(pkgs))
}

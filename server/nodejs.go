package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strconv"
	"strings"

	"github.com/ije/gox/utils"
)

const (
	nodejsMinVersion = 20
	nodejsLatestLTS  = "20.11.1"
	nodeTypesVersion = "20.11.20"
	denoStdVersion   = "0.177.1"
)

var nodejsInternalModules = map[string]bool{
	"assert":              true,
	"assert/strict":       true,
	"async_hooks":         true,
	"child_process":       true,
	"cluster":             true,
	"buffer":              true,
	"console":             true,
	"constants":           true,
	"crypto":              true,
	"dgram":               true,
	"diagnostics_channel": true,
	"dns":                 true,
	"domain":              true,
	"events":              true,
	"fs":                  true,
	"fs/promises":         true,
	"http":                true,
	"http2":               true,
	"https":               true,
	"inspector":           true,
	"module":              true,
	"net":                 true,
	"os":                  true,
	"path":                true,
	"path/posix":          true,
	"path/win32":          true,
	"perf_hooks":          true,
	"process":             true,
	"punycode":            true,
	"querystring":         true,
	"readline":            true,
	"repl":                true,
	"stream":              true,
	"stream/promises":     true,
	"stream/web":          true,
	"string_decoder":      true,
	"sys":                 true,
	"timers":              true,
	"timers/promises":     true,
	"tls":                 true,
	"trace_events":        true,
	"tty":                 true,
	"url":                 true,
	"util":                true,
	"util/types":          true,
	"v8":                  true,
	"vm":                  true,
	"wasi":                true,
	"webcrypto":           true,
	"worker_threads":      true,
	"zlib":                true,
}

// copy from https://github.com/webpack/webpack/blob/master/lib/ModuleNotFoundError.js#L13
var polyfilledInternalNodeModules = map[string]string{
	"assert":         "assert@2.1.0",
	"buffer":         "buffer@6.0.3",
	"console":        "console-browserify@1.2.0",
	"constants":      "constants-browserify@1.0.0",
	"crypto":         "crypto-browserify@3.12.0",
	"domain":         "domain-browser@5.2.0",
	"http":           "stream-http@3.2.0",
	"https":          "https-browserify@1.0.0",
	"os":             "os-browserify@0.3.0/browser",
	"path":           "path-browserify@1.0.1",
	"punycode":       "punycode@2.3.1",
	"querystring":    "querystring-es3@0.2.1",
	"stream":         "stream-browserify@3.0.0",
	"stream/web":     "web-streams-polyfill@3.2.1",
	"string_decoder": "string_decoder@1.3.0",
	"sys":            "util@0.12.5",
	"timers":         "timers-browserify@2.0.12",
	"tty":            "tty-browserify@0.0.1",
	"util":           "util@0.12.5",
	"vm":             "vm-browserify@1.1.2",
	"zlib":           "browserify-zlib@0.2.0",
}

var denoNextUnspportedNodeModules = map[string]bool{
	"inspector": true,
}

func checkNodejs(installDir string) (nodeVersion string, pnpmVersion string, err error) {
	nodeVersion, major, err := getNodejsVersion()
	usingSystemNodejs := err == nil && major >= nodejsMinVersion

	if !usingSystemNodejs {
		PATH := os.Getenv("PATH")
		nodeBinDir := path.Join(installDir, "bin")
		if !strings.Contains(PATH, nodeBinDir) {
			os.Setenv("PATH", fmt.Sprintf("%s%c%s", nodeBinDir, os.PathListSeparator, PATH))
		}
		nodeVersion, major, err = getNodejsVersion()
		if err != nil || major < nodejsMinVersion {
			err = installNodejs(installDir, nodejsLatestLTS)
			if err != nil {
				return
			}
			log.Infof("nodejs %s installed", nodejsLatestLTS)
		}
		nodeVersion, major, err = getNodejsVersion()
	}
	if err == nil && major < nodejsMinVersion {
		err = fmt.Errorf("bad nodejs version %s need %d+", nodeVersion, nodejsMinVersion)
	}
	if err != nil {
		return
	}

	pnpmOutput, err := exec.Command("pnpm", "-v").CombinedOutput()
	if err != nil && errors.Is(err, exec.ErrNotFound) {
		out, e := exec.Command("npm", "install", "pnpm", "-g").CombinedOutput()
		if e != nil {
			err = fmt.Errorf("failed to install pnpm: %v", string(out))
			return
		}
		pnpmOutput, err = exec.Command("pnpm", "-v").CombinedOutput()
	}
	if err == nil {
		pnpmVersion = strings.TrimSpace(string(pnpmOutput))
	}
	return
}

func getNodejsVersion() (version string, major int, err error) {
	output, err := exec.Command("node", "--version").CombinedOutput()
	if err != nil {
		return
	}

	version = strings.TrimPrefix(strings.TrimSpace(string(output)), "v")
	s, _ := utils.SplitByFirstByte(version, '.')
	major, err = strconv.Atoi(s)
	return
}

func installNodejs(installDir string, version string) (err error) {
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "x64"
	case "386":
		arch = "x86"
	}
	dlURL := fmt.Sprintf("https://nodejs.org/dist/v%s/node-v%s-%s-%s.tar.xz", version, version, runtime.GOOS, arch)
	resp, err := http.Get(dlURL)
	if err != nil {
		err = fmt.Errorf("download nodejs: %v", err)
		return
	}
	defer resp.Body.Close()

	savePath := path.Join(os.TempDir(), path.Base(dlURL))
	f, err := os.Create(savePath)
	if err != nil {
		return
	}
	io.Copy(f, resp.Body)
	f.Close()

	cmd := exec.Command("tar", "-xJf", path.Base(dlURL))
	cmd.Dir = os.TempDir()
	output, err := cmd.CombinedOutput()
	if err != nil {
		if len(output) > 0 {
			err = errors.New(string(output))
		}
		return
	}

	// remove old installation if exists
	os.RemoveAll(installDir)

	cmd = exec.Command("mv", "-f", strings.TrimSuffix(path.Base(dlURL), ".tar.xz"), installDir)
	cmd.Dir = os.TempDir()
	output, err = cmd.CombinedOutput()
	if err != nil {
		if len(output) > 0 {
			err = errors.New(string(output))
		}
	}
	return
}

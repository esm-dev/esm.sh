package server

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strconv"
	"strings"

	"github.com/ije/gox/utils"
)

var builtInNodeModules = map[string]bool{
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
var polyfilledBuiltInNodeModules = map[string]string{
	"assert":         "assert@2.0.0",
	"buffer":         "buffer@6.0.3",
	"console":        "console-browserify@1.2.0",
	"constants":      "constants-browserify@1.0.0",
	"crypto":         "crypto-browserify@3.12.0",
	"domain":         "domain-browser@4.22.0",
	"http":           "stream-http@3.2.0",
	"https":          "https-browserify@1.0.0",
	"os":             "os-browserify@0.3.0/browser",
	"path":           "path-browserify@1.0.1",
	"punycode":       "punycode@2.3.0",
	"querystring":    "querystring-es3@0.2.1",
	"stream":         "stream-browserify@3.0.0",
	"stream/web":     "web-streams-polyfill@3.2.1",
	"string_decoder": "string_decoder@1.3.0",
	"sys":            "util@0.12.5",
	"timers":         "timers-browserify@2.0.12",
	"tty":            "tty-browserify@0.0.1",
	"url":            "url@0.11.0",
	"util":           "util@0.12.5",
	"vm":             "vm-browserify@1.1.2",
	"zlib":           "browserify-zlib@0.2.0",
}

func checkNodejs(installDir string) (nodeVer string, pnpmVer string, err error) {
	var installed bool
CheckNodejs:
	nodeVer, major, err := getNodejsVersion()
	if err != nil || major < nodejsMinVersion {
		PATH := os.Getenv("PATH")
		nodeBinDir := path.Join(installDir, "bin")
		if !strings.Contains(PATH, nodeBinDir) {
			os.Setenv("PATH", fmt.Sprintf("%s%c%s", nodeBinDir, os.PathListSeparator, PATH))
			goto CheckNodejs
		} else if !installed {
			err = os.RemoveAll(installDir)
			if err != nil {
				return
			}
			err = installNodejs(installDir, nodejsLatestLTS)
			if err != nil {
				return
			}
			log.Infof("nodejs %s installed", nodejsLatestLTS)
			installed = true
			goto CheckNodejs
		} else {
			if err == nil {
				err = fmt.Errorf("bad nodejs version %s need %d+", nodeVer, nodejsMinVersion)
			}
			return
		}
	}

CheckPnpm:
	output, err := exec.Command("pnpm", "-v").CombinedOutput()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			output, err = exec.Command("npm", "install", "pnpm", "-g").CombinedOutput()
			if err != nil {
				err = fmt.Errorf("install pnpm: %s", strings.TrimSpace(string(output)))
				return
			}
			goto CheckPnpm
		}
		err = fmt.Errorf("bad pnpm version: %s", strings.TrimSpace(string(output)))
	}
	if err == nil {
		pnpmVer = strings.TrimSpace(string(output))
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

func installNodejs(dir string, version string) (err error) {
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "x64"
	case "386":
		arch = "x86"
	}
	dlURL := fmt.Sprintf("https://nodejs.org/dist/v%s/node-v%s-%s-%s.tar.xz", version, version, runtime.GOOS, arch)
	resp, err := fetch(dlURL)
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

	cmd = exec.Command("mv", "-f", strings.TrimSuffix(path.Base(dlURL), ".tar.xz"), dir)
	cmd.Dir = os.TempDir()
	output, err = cmd.CombinedOutput()
	if err != nil {
		if len(output) > 0 {
			err = errors.New(string(output))
		}
	}
	return
}

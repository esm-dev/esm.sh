package server

import (
	"archive/tar"
	"compress/gzip"
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
	nodejsMinVersion = 22
	nodeTypesVersion = "22.9.0"
	pnpmMinVersion   = "9.0.0"
)

var nodeInternalModules = map[string]bool{
	"assert":              true,
	"assert/strict":       true,
	"async_hooks":         true,
	"buffer":              true,
	"child_process":       true,
	"cluster":             true,
	"console":             true,
	"constants":           true,
	"crypto":              true,
	"dgram":               true,
	"diagnostics_channel": true,
	"dns":                 true,
	"dns/promises":        true,
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
	"stream/consumers":    true,
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

func normalizeImportSpecifier(specifier string) string {
	specifier = strings.TrimPrefix(specifier, "npm:")
	specifier = strings.TrimPrefix(specifier, "./node_modules/")
	if specifier == "." {
		specifier = "./index"
	} else if specifier == ".." {
		specifier = "../index"
	}
	if nodeInternalModules[specifier] {
		return "node:" + specifier
	}
	return specifier
}

func isNodeInternalModule(specifier string) bool {
	return strings.HasPrefix(specifier, "node:") && nodeInternalModules[specifier[5:]]
}

func checkNodejs(installDir string) (nodeVersion string, pnpmVersion string, err error) {
	nodeVersion, major, err := lookupSystemNodejs()
	useSystemNodejs := err == nil && major >= nodejsMinVersion

	if !useSystemNodejs {
		PATH := os.Getenv("PATH")
		nodeBinDir := path.Join(installDir, "bin")
		if !strings.Contains(PATH, nodeBinDir) {
			os.Setenv("PATH", fmt.Sprintf("%s%c%s", PATH, os.PathListSeparator, nodeBinDir))
		}
		nodeVersion, major, err = lookupSystemNodejs()
		if err != nil || major < nodejsMinVersion {
			var latestVersion string
			latestVersion, err = getNodejsLatestVersion()
			if err != nil {
				return
			}
			err = installNodejs(installDir, latestVersion)
			if err != nil {
				return
			}
			log.Infof("nodejs %s installed", latestVersion)
		}
		nodeVersion, major, err = lookupSystemNodejs()
	}
	if err == nil && major < nodejsMinVersion {
		err = fmt.Errorf("bad nodejs version %s, needs %d+", nodeVersion, nodejsMinVersion)
	}
	if err != nil {
		return
	}

	pnpmOutput, err := run("pnpm", "-v")
	if (err != nil && errors.Is(err, exec.ErrNotFound)) || (err == nil && semverLessThan(strings.TrimSpace(string(pnpmOutput)), pnpmMinVersion)) {
		_, err = run("npm", "install", "pnpm", "-g")
		if err != nil {
			return
		}
		pnpmOutput, err = run("pnpm", "-v")
	}
	if err == nil {
		pnpmVersion = strings.TrimSpace(string(pnpmOutput))
	}
	return
}

func lookupSystemNodejs() (version string, major int, err error) {
	output, err := run("node", "--version")
	if err != nil {
		return
	}

	version = strings.TrimPrefix(strings.TrimSpace(string(output)), "v")
	s, _ := utils.SplitByFirstByte(version, '.')
	major, err = strconv.Atoi(s)
	return
}

func getNodejsLatestVersion() (verison string, err error) {
	var res *http.Response
	res, err = http.Get(fmt.Sprintf("https://nodejs.org/download/release/latest-v%d.x/", nodejsMinVersion))
	if err != nil {
		return
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		err = fmt.Errorf("http.get: %s", http.StatusText(res.StatusCode))
		return
	}
	var body []byte
	body, err = io.ReadAll(res.Body)
	if err != nil {
		return
	}
	i := strings.Index(string(body), fmt.Sprintf("node-v%d.", nodejsMinVersion))
	if i < 0 {
		err = fmt.Errorf("no nodejs version found")
		return
	}
	verison, _ = utils.SplitByFirstByte(string(body[i+5:]), '-')
	return
}

func installNodejs(installDir string, version string) (err error) {
	arch := runtime.GOARCH
	goos := runtime.GOOS
	switch arch {
	case "arm64":
		arch = "arm64"
	case "amd64":
		arch = "x64"
	case "386":
		arch = "x86"
	}

	if goos == "windows" {
		err = fmt.Errorf("download nodejs: doesn't support windows yet")
		return
	}

	dlURL := fmt.Sprintf("https://nodejs.org/dist/%s/node-%s-%s-%s.tar.gz", version, version, goos, arch)
	fmt.Println("Downloading", dlURL, "...")
	resp, err := http.Get(dlURL)
	if err != nil {
		err = fmt.Errorf("download nodejs: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		err = fmt.Errorf("download nodejs: %s", http.StatusText(resp.StatusCode))
		return
	}

	defer func() {
		if err != nil {
			os.RemoveAll(installDir)
			err = fmt.Errorf("extract %s: %v", path.Base(dlURL), err)
		}
	}()

	// extract
	gr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	for {
		var header *tar.Header
		header, err = tr.Next()
		if err == io.EOF {
			err = nil
			break
		}
		if err == nil {
			filePath := path.Join(installDir, header.Name)
			if header.Typeflag == tar.TypeDir {
				err = os.MkdirAll(filePath, 0755)
			} else if header.Typeflag == tar.TypeReg {
				var file *os.File
				file, err = os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY, os.FileMode(header.Mode))
				if err == nil {
					_, err = io.Copy(file, tr)
					file.Close()
				}
			}
		}
		if err != nil {
			break
		}
	}
	return
}

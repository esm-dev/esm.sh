package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"esm.sh/server/storage"

	"github.com/ije/gox/utils"
	"github.com/postui/postdb"
)

const (
	minNodejsVersion = 14
	nodejsLatestLTS  = "14.17.5"
	nodejsDistURL    = "https://nodejs.org/dist/"
	refreshDuration  = 10 * 60 // 10 minues
)

var builtInNodeModules = map[string]bool{
	"assert":              true,
	"async_hooks":         true,
	"child_process":       true,
	"cluster":             true,
	"buffer":              true,
	"console":             true,
	"constants":           true,
	"crypto":              true,
	"dgram":               true,
	"dns":                 true,
	"domain":              true,
	"events":              true,
	"fs":                  true,
	"http":                true,
	"http2":               true,
	"https":               true,
	"inspector":           true,
	"module":              true,
	"net":                 true,
	"os":                  true,
	"path":                true,
	"perf_hooks":          true,
	"process":             true,
	"punycode":            true,
	"querystring":         true,
	"readline":            true,
	"repl":                true,
	"stream":              true,
	"_stream_duplex":      true,
	"_stream_passthrough": true,
	"_stream_readable":    true,
	"_stream_transform":   true,
	"_stream_writable":    true,
	"string_decoder":      true,
	"sys":                 true,
	"timers":              true,
	"tls":                 true,
	"trace_events":        true,
	"tty":                 true,
	"url":                 true,
	"util":                true,
	"v8":                  true,
	"vm":                  true,
	"worker_threads":      true,
	"zlib":                true,
}

// status: https://deno.land/std/node
var denoStdNodeModules = map[string]bool{
	"fs":            true,
	"child_process": true,
	"path":          true,
	"querystring":   true,
	"timers":        true,
	"url":           true,
}

// copy from https://github.com/webpack/webpack/blob/master/lib/ModuleNotFoundError.js#L13
var polyfilledBuiltInNodeModules = map[string]string{
	"assert":              "assert",
	"buffer":              "buffer",
	"console":             "console-browserify",
	"constants":           "constants-browserify",
	"crypto":              "crypto-browserify",
	"domain":              "domain-browser",
	"events":              "events",
	"http":                "stream-http",
	"https":               "https-browserify",
	"os":                  "os-browserify/browser",
	"path":                "path-browserify",
	"punycode":            "punycode",
	"process":             "process/browser",
	"querystring":         "querystring-es3",
	"stream":              "stream-browserify",
	"_stream_duplex":      "readable-stream/duplex",
	"_stream_passthrough": "readable-stream/passthrough",
	"_stream_readable":    "readable-stream/readable",
	"_stream_transform":   "readable-stream/transform",
	"_stream_writable":    "readable-stream/writable",
	"string_decoder":      "string_decoder",
	"sys":                 "util",
	"timers":              "timers-browserify",
	"tty":                 "tty-browserify",
	"url":                 "url",
	"util":                "util",
	"vm":                  "vm-browserify",
	"zlib":                "browserify-zlib",
}

// NpmPackageRecords defines version records of a npm package
type NpmPackageRecords struct {
	DistTags map[string]string     `json:"dist-tags"`
	Versions map[string]NpmPackage `json:"versions"`
}

// NpmPackage defines the package.json of npm
type NpmPackage struct {
	Name             string            `json:"name"`
	Version          string            `json:"version"`
	Main             string            `json:"main,omitempty"`
	Module           string            `json:"module,omitempty"`
	Type             string            `json:"type,omitempty"`
	Types            string            `json:"types,omitempty"`
	Typings          string            `json:"typings,omitempty"`
	Dependencies     map[string]string `json:"dependencies,omitempty"`
	PeerDependencies map[string]string `json:"peerDependencies,omitempty"`
	DefinedExports   interface{}       `json:"exports,omitempty"`
}

// Node defines the nodejs info
type Node struct {
	version     string
	npmRegistry string
}

func checkNode(nodePrefix string) (node *Node, err error) {
	var installed bool
CheckNodejs:
	version, major, err := getNodejsVersion()
	if err != nil || major < minNodejsVersion {
		PATH := os.Getenv("PATH")
		nodeBinDir := path.Join(nodePrefix, "bin")
		if !strings.Contains(PATH, nodeBinDir) {
			os.Setenv("PATH", fmt.Sprintf("%s%c%s", nodeBinDir, os.PathListSeparator, PATH))
			goto CheckNodejs
		} else if !installed {
			err = os.RemoveAll(nodePrefix)
			if err != nil {
				return
			}
			err = installNodejs(nodePrefix, nodejsLatestLTS)
			if err != nil {
				return
			}
			log.Infof("nodejs %s installed", nodejsLatestLTS)
			installed = true
			goto CheckNodejs
		} else {
			if err == nil {
				err = fmt.Errorf("bad nodejs version %s need %d+", version, minNodejsVersion)
			}
			return
		}
	}

	node = &Node{
		version:     version,
		npmRegistry: "https://registry.npmjs.org/",
	}

	output, err := exec.Command("npm", "config", "get", "registry").CombinedOutput()
	if err == nil {
		node.npmRegistry = strings.TrimRight(strings.TrimSpace(string(output)), "/") + "/"
	}

CheckYarn:
	output, err = exec.Command("yarn", "-v").CombinedOutput()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			output, err = exec.Command("npm", "install", "yarn", "-g").CombinedOutput()
			if err != nil {
				err = errors.New("install yarn: " + strings.TrimSpace(string(output)))
				return
			}
			goto CheckYarn
		}
		err = errors.New("bad yarn version")
	}
	return
}

func (node *Node) getPackageInfo(wd string, name string, version string) (info NpmPackage, submodule string, formPackageJSON bool, err error) {
	slice := strings.Split(name, "/")
	if l := len(slice); strings.HasPrefix(name, "@") && l > 1 {
		name = strings.Join(slice[:2], "/")
		if l > 2 {
			submodule = strings.Join(slice[2:], "/")
		}
	} else {
		name = slice[0]
		if l > 1 {
			submodule = strings.Join(slice[1:], "/")
		}
	}

	if wd != "" {
		pkgJsonPath := path.Join(wd, "node_modules", name, "package.json")
		if fileExists(pkgJsonPath) {
			err = utils.ParseJSONFile(pkgJsonPath, &info)
			if err == nil {
				formPackageJSON = true
				return
			}
		}
	}

	version = resolveVersion(version)
	isFullVersion := regFullVersion.MatchString(version)
	key := fmt.Sprintf("npm:%s@%s", name, version)
	store, modtime, err := db.Get(key)
	if err == nil {
		if isFullVersion || int64(modtime.Unix())+refreshDuration > time.Now().Unix() {
			if json.Unmarshal([]byte(store["package"]), &info) == nil {
				return
			}
		}
	}
	if err != nil && err != postdb.ErrNotFound {
		return
	}

	start := time.Now()
	resp, err := httpClient.Get(node.npmRegistry + name)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 || resp.StatusCode == 401 {
		err = fmt.Errorf("npm: package '%s' not found", name)
		return
	}
	if resp.StatusCode != 200 {
		ret, _ := ioutil.ReadAll(resp.Body)
		err = fmt.Errorf("npm: can't get metadata of package '%s' (%s: %s)", name, resp.Status, string(ret))
		return
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err == io.EOF {
		err = nil
	}
	if err != nil {
		return
	}

	var h NpmPackageRecords
	err = json.Unmarshal(data, &h)
	if err != nil {
		return
	}

	if isFullVersion {
		info = h.Versions[version]
	} else {
		distVersion, ok := h.DistTags[version]
		if ok {
			info = h.Versions[distVersion]
		} else {
			var majorVerions versionSlice
			for key := range h.Versions {
				if regFullVersion.MatchString(key) && strings.HasPrefix(key, version+".") {
					majorVerions = append(majorVerions, key)
				}
			}
			if l := len(majorVerions); l > 0 {
				if l > 1 {
					sort.Sort(majorVerions)
				}
				info = h.Versions[majorVerions[0]]
			}
		}
	}

	if info.Version == "" {
		err = fmt.Errorf("npm: version '%s' not found", version)
		return
	}

	log.Debugf("get npm package(%s@%s) info in %v", name, info.Version, time.Now().Sub(start))

	// cache data
	db.Put(key, storage.Store{"package": string(utils.MustEncodeJSON(info))})
	return
}

func resolveVersion(version string) string {
	if version == "*" {
		return "latest"
	}
	if strings.ContainsRune(version, '>') || strings.ContainsRune(version, '<') {
		return "latest"
	}
	for _, p := range []string{"||", " - "} {
		if strings.Contains(version, p) {
			a := sort.StringSlice(strings.Split(version, p))
			vs := make(versionSlice, len(a))
			for i, v := range a {
				version := resolveVersion(strings.TrimSpace(v))
				vs[i] = version
			}
			sort.Sort(vs)
			version = vs[0]
		}
	}

	if strings.HasSuffix(version, ".x") {
		version = strings.TrimSuffix(version, ".x")
	}
	if strings.HasPrefix(version, "=") {
		version = strings.TrimPrefix(version, "=")
	} else if strings.HasPrefix(version, "^") {
		version, _ = utils.SplitByFirstByte(version[1:], '.')
	} else if strings.HasPrefix(version, "~") {
		major, rest := utils.SplitByFirstByte(version[1:], '.')
		minor, _ := utils.SplitByFirstByte(rest, '.')
		version = major + "." + minor
	}
	return version
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

// see https://nodejs.org/api/packages.html
func useDefinedExports(p *NpmPackage, exports interface{}) {
	s, ok := exports.(string)
	if ok {
		if p.Type == "module" && p.Module == "" {
			p.Module = s
		} else if p.Main == "" {
			p.Main = s
		}
		return
	}

	m, ok := exports.(map[string]interface{})
	if ok {
		for _, key := range []string{"import", "module", "browser"} {
			value, ok := m[key]
			if ok {
				s, ok := value.(string)
				if ok && s != "" {
					p.Module = s
					break
				}
			}
		}
		for _, key := range []string{"require", "node", "default"} {
			value, ok := m[key]
			if ok {
				s, ok := value.(string)
				if ok && s != "" {
					p.Main = s
					break
				}
			}
		}
		for key, value := range m {
			s, ok := value.(string)
			if ok && s != "" {
				switch key {
				case "types":
					p.Types = s
				case "typings":
					p.Typings = s
				}
			}
		}
	}
}

func fixNpmPackage(p NpmPackage) *NpmPackage {
	np := &p

	if p.Module == "" && p.DefinedExports != nil {
		useDefinedExports(np, p.DefinedExports)
		if m, ok := p.DefinedExports.(map[string]interface{}); ok {
			v, ok := m["."]
			if ok {
				useDefinedExports(np, v)
			}
		}
	}

	if p.Module == "" && p.Main != "" && (p.Type == "module" || strings.HasSuffix(p.Main, ".mjs")) {
		p.Module = p.Main
	}

	return np
}

func installNodejs(dir string, version string) (err error) {
	dlURL := fmt.Sprintf("%sv%s/node-v%s-%s-x64.tar.xz", nodejsDistURL, version, version, runtime.GOOS)
	log.Debugf("downloading %s", dlURL)
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

func yarnAdd(wd string, packages ...string) (err error) {
	if len(packages) > 0 {
		start := time.Now()
		args := []string{
			"add",
			"--non-interactive",
			"--no-progress",
			"--no-bin-links",
			"--ignore-scripts",
			"--ignore-platform",
			"--ignore-engines",
		}
		if config.yarnCacheDir != "" {
			args = append(args, "--cache-folder", config.yarnCacheDir)
		}
		cmd := exec.Command("yarn", append(args, packages...)...)
		cmd.Dir = wd
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("yarn add %s: %s", strings.Join(packages, " "), string(output))
		}
		log.Debug("yarn add", strings.Join(packages, " "), "in", time.Now().Sub(start))
	}
	return
}

// provided by @jimisaacs
func transformPackageNameToTypesPackage(pkgName string) string {
	if strings.HasPrefix(pkgName, "@") {
		pkgName = strings.Replace(pkgName[1:], "/", "__", 1)
	}
	return path.Join("@types", pkgName)
}

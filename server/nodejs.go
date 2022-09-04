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
	"sync"
	"time"

	"esm.sh/server/storage"

	"github.com/Masterminds/semver/v3"
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
	"assert":         "assert",
	"buffer":         "buffer",
	"console":        "console-browserify",
	"constants":      "constants-browserify",
	"crypto":         "crypto-browserify",
	"domain":         "domain-browser",
	"events":         "events",
	"http":           "stream-http",
	"https":          "https-browserify",
	"os":             "os-browserify/browser",
	"path":           "path-browserify",
	"punycode":       "punycode",
	"process":        "process/browser",
	"querystring":    "querystring-es3",
	"stream":         "stream-browserify",
	"stream/web":     "web-streams-polyfill",
	"string_decoder": "string_decoder",
	"sys":            "util",
	"timers":         "timers-browserify",
	"tty":            "tty-browserify",
	"url":            "url",
	"util":           "util",
	"vm":             "vm-browserify",
	"zlib":           "browserify-zlib",
}

// status: https://deno.land/std/node
var denoStdNodeModules = map[string]bool{
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
	"process":             true,
	"path":                true,
	"path/posix":          true,
	"path/win32":          true,
	"perf_hooks":          true,
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

// NpmPackageVerions defines versions of a npm package
type NpmPackageVerions struct {
	DistTags map[string]string     `json:"dist-tags"`
	Versions map[string]NpmPackage `json:"versions"`
}

// NpmPackage defines the package.json of npm
type NpmPackage struct {
	Name             string                 `json:"name"`
	Version          string                 `json:"version"`
	Main             string                 `json:"main,omitempty"`
	Module           string                 `json:"module,omitempty"`
	JsNextMain       string                 `json:"jsnext:main,omitempty"`
	ES2015           string                 `json:"es2015,omitempty"`
	Type             string                 `json:"type,omitempty"`
	Types            string                 `json:"types,omitempty"`
	Typings          string                 `json:"typings,omitempty"`
	Dependencies     map[string]string      `json:"dependencies,omitempty"`
	PeerDependencies map[string]string      `json:"peerDependencies,omitempty"`
	Imports          map[string]interface{} `json:"imports,omitempty"`
	DefinedExports   interface{}            `json:"exports,omitempty"`
}

// Node defines the nodejs info
type Node struct {
	version     string
	npmRegistry string
	yarn        string
}

func checkNode(installDir string, npmRegistry string) (node *Node, err error) {
	var installed bool
CheckNodejs:
	version, major, err := getNodejsVersion()
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
				err = fmt.Errorf("bad nodejs version %s need %d+", version, nodejsMinVersion)
			}
			return
		}
	}

	node = &Node{
		version:     version,
		npmRegistry: "https://registry.npmjs.org/",
	}
	var output []byte
	if npmRegistry != "" {
		node.npmRegistry = npmRegistry
	} else {
		output, err := exec.Command("npm", "config", "get", "registry").CombinedOutput()
		if err == nil {
			node.npmRegistry = strings.TrimRight(strings.TrimSpace(string(output)), "/") + "/"
		}
	}

CheckYarn:
	output, err = exec.Command("yarn", "-v").CombinedOutput()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			output, err = exec.Command("npm", "install", "yarn", "-g").CombinedOutput()
			if err != nil {
				err = fmt.Errorf("install yarn: %s", strings.TrimSpace(string(output)))
				return
			}
			goto CheckYarn
		}
		err = fmt.Errorf("bad yarn version: %s", strings.TrimSpace(string(output)))
	}
	if err == nil {
		node.yarn = strings.TrimSpace(string(output))
	}
	return
}

func getPackageInfo(wd string, name string, version string) (info NpmPackage, submodule string, fromPackageJSON bool, err error) {
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

	if name == "@types/node" {
		info = NpmPackage{
			Name:    "@types/node",
			Version: nodeTypesVersion,
			Types:   "index.d.ts",
		}
		return
	}

	if wd != "" {
		pkgJsonPath := path.Join(wd, "node_modules", name, "package.json")
		if fileExists(pkgJsonPath) && utils.ParseJSONFile(pkgJsonPath, &info) == nil {
			info, err = fixPkgVersion(info)
			if err == nil {
				fromPackageJSON = true
				return
			}
		}
	}

	info, err = fetchPackageInfo(name, version)
	if err == nil {
		info, err = fixPkgVersion(info)
	}
	return
}

var lock sync.Map

func fetchPackageInfo(name string, version string) (info NpmPackage, err error) {
	if version == "" {
		version = "latest"
	}
	id := fmt.Sprintf("npm:%s@%s", name, version)

	// wait lock release
	for {
		_, ok := lock.Load(id)
		if !ok {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	data, err := cache.Get(id)
	if err == nil && json.Unmarshal(data, &info) == nil {
		return
	}
	if err != nil && err != storage.ErrNotFound && err != storage.ErrExpired {
		log.Error("cache:", err)
	}

	lock.Store(id, struct{}{})
	defer lock.Delete(id)

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

	data, err = ioutil.ReadAll(resp.Body)
	if err == io.EOF {
		err = nil
	}
	if err != nil {
		return
	}

	var h NpmPackageVerions
	err = json.Unmarshal(data, &h)
	if err != nil {
		return
	}

	if len(h.Versions) == 0 {
		err = fmt.Errorf("npm: versions of %s not found", name)
		return
	}

	isFullVersion := regFullVersion.MatchString(version)
	if isFullVersion {
		info = h.Versions[version]
	} else {
		distVersion, ok := h.DistTags[version]
		if ok {
			info = h.Versions[distVersion]
		} else {
			var c *semver.Constraints
			c, err = semver.NewConstraint(version)
			if err != nil && version != "latest" {
				return fetchPackageInfo(name, "latest")
			}
			vs := make([]*semver.Version, len(h.Versions))
			i := 0
			for v := range h.Versions {
				// ignore prerelease versions
				if !strings.ContainsRune(version, '-') && strings.ContainsRune(v, '-') {
					continue
				}
				var ver *semver.Version
				ver, err = semver.NewVersion(v)
				if err != nil {
					return
				}
				if c.Check(ver) {
					vs[i] = ver
					i++
				}
			}
			if i > 0 {
				vs = vs[:i]
				if i > 1 {
					sort.Sort(semver.Collection(vs))
				}
				info = h.Versions[vs[i-1].String()]
			}
		}
	}

	if info.Version == "" {
		err = fmt.Errorf("npm: version '%s' of %s not found", version, name)
		return
	}

	log.Debugf("lookup package(%s@%s) in %v", name, info.Version, time.Since(start))

	// cache data
	var ttl time.Duration = 0
	if !isFullVersion {
		ttl = 10 * time.Minute
	}
	cache.Set(id, utils.MustEncodeJSON(info), ttl)
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

// see https://nodejs.org/api/packages.html
func resolvePackageExports(p *NpmPackage, exports interface{}, target string, isDev bool, pType string) {
	s, ok := exports.(string)
	if ok {
		if pType == "module" {
			p.Module = s
		} else {
			p.Main = s
		}
		return
	}

	m, ok := exports.(map[string]interface{})
	if ok {
		names := []string{"es2015", "module", "import", "browser", "worker"}
		if target == "deno" {
			names = []string{"deno", "es2015", "module", "import", "worker", "browser"}
		}
		if p.Type == "module" {
			if isDev {
				names = append([]string{"development"}, names...)
			}
			names = append(names, "default")
		}
		// support solid.js ssr in deno
		if p.Name == "solid-js" && target == "deno" {
			names = append([]string{"node"}, names...)
		}
		for _, name := range names {
			value, ok := m[name]
			if ok {
				resolvePackageExports(p, value, target, isDev, "module")
				break
			}
		}
		for _, name := range []string{"require", "node", "default"} {
			value, ok := m[name]
			if ok {
				resolvePackageExports(p, value, target, isDev, "")
				break
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

func fixNpmPackage(p NpmPackage, target string, isDev bool) *NpmPackage {
	exports := p.DefinedExports
	np := &p

	if exports != nil {
		if m, ok := exports.(map[string]interface{}); ok {
			v, ok := m["."]
			if ok {
				/*
					exports: {
						".": {
							"require": "./cjs/index.js",
							"import": "./esm/index.js"
						}
					}
					exports: {
						".": "./esm/index.js"
					}
				*/
				resolvePackageExports(np, v, target, isDev, np.Type)
			} else {
				/*
					exports: {
						"require": "./cjs/index.js",
						"import": "./esm/index.js"
					}
				*/
				resolvePackageExports(np, m, target, isDev, np.Type)
			}
		} else if _, ok := exports.(string); ok {
			/*
			  exports: "./esm/index.js"
			*/
			resolvePackageExports(np, exports, target, isDev, np.Type)
		}
	}

	if p.Module == "" {
		if p.JsNextMain != "" {
			p.Module = p.JsNextMain
		} else if p.ES2015 != "" {
			p.Module = p.ES2015
		} else if p.Main != "" && (p.Type == "module" || strings.Contains(p.Main, "/esm/") || strings.Contains(p.Main, "/es/") || strings.HasSuffix(p.Main, ".mjs")) {
			p.Module = p.Main
		}
	}

	if p.Types == "" && p.Typings != "" {
		p.Types = p.Typings
	}

	return np
}

func installNodejs(dir string, version string) (err error) {
	dlURL := fmt.Sprintf("https://nodejs.org/dist/v%s/node-v%s-%s-x64.tar.xz", version, version, runtime.GOOS)
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
			"--check-files",
			"--ignore-engines",
			"--ignore-platform",
			"--ignore-scripts",
			"--ignore-workspace-root-check",
			"--no-bin-links",
			"--no-default-rc",
			"--no-lockfile",
			"--no-node-version-check",
			"--no-progress",
			"--non-interactive",
			"--silent",
			"--registry=" + node.npmRegistry,
		}
		yarnCacheDir := os.Getenv("YARN_CACHE_DIR")
		if yarnCacheDir != "" {
			args = append(args, "--cache-folder", yarnCacheDir)
		}
		yarnMutex := os.Getenv("YARN_MUTEX")
		if yarnMutex != "" {
			args = append(args, "--mutex", yarnMutex)
		}
		cmd := exec.Command("yarn", append(args, packages...)...)
		cmd.Dir = wd
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("yarn add %s: %s", strings.Join(packages, ","), string(output))
		}
		log.Debug("yarn add", strings.Join(packages, ","), "in", time.Since(start))
	}
	return
}

func yarnCacheClean(wd string, packages ...string) {
	if len(packages) > 0 {
		args := []string{"cache", "clean"}
		yarnCacheDir := os.Getenv("YARN_CACHE_DIR")
		if yarnCacheDir != "" {
			args = append(args, "--cache-folder", yarnCacheDir)
		}
		yarnMutex := os.Getenv("YARN_MUTEX")
		if yarnMutex != "" {
			args = append(args, "--mutex", yarnMutex)
		}
		cmd := exec.Command("yarn", append(args, packages...)...)
		cmd.Dir = wd
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Warnf("yarn cache clean %s: %s", strings.Join(packages, ","), err)
		} else {
			log.Debugf("yarn cache clean %s: %s", strings.Join(packages, ","), string(output))
		}
	}
}

// added by @jimisaacs
func toTypesPackageName(pkgName string) string {
	if strings.HasPrefix(pkgName, "@") {
		pkgName = strings.Replace(pkgName[1:], "/", "__", 1)
	}
	return "@types/" + pkgName
}

var fixedPkgVersions = map[string]string{
	"@types/react@17": "17.0.49",
	"@types/react@18": "18.0.18",
}

func fixPkgVersion(info NpmPackage) (NpmPackage, error) {
	for prefix, ver := range fixedPkgVersions {
		if strings.HasPrefix(info.Name+"@"+info.Version, prefix) {
			return fetchPackageInfo(info.Name, ver)
		}
	}
	return info, nil
}

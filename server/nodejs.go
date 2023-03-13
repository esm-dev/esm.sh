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

	"github.com/esm-dev/esm.sh/server/storage"

	"github.com/Masterminds/semver/v3"
	"github.com/ije/gox/utils"
	"github.com/ije/gox/valid"
)

// ref https://github.com/npm/validate-npm-package-name
var npmNaming = valid.Validator{valid.FromTo{'a', 'z'}, valid.FromTo{'A', 'Z'}, valid.FromTo{'0', '9'}, valid.Eq('.'), valid.Eq('-'), valid.Eq('_')}

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

var denoUnspportedNodeModules = map[string]bool{
	"inspector": true,
}

// @ref https://deno.land/std@0.177.0/node
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

// NpmPackageTemp defines the package.json of npm
type NpmPackageTemp struct {
	Name             string                 `json:"name"`
	Version          string                 `json:"version"`
	Type             string                 `json:"type,omitempty"`
	Main             string                 `json:"main,omitempty"`
	Browser          StringOrMap            `json:"browser,omitempty"`
	Module           StringOrMap            `json:"module,omitempty"`
	ES2015           StringOrMap            `json:"es2015,omitempty"`
	JsNextMain       string                 `json:"jsnext:main,omitempty"`
	Types            string                 `json:"types,omitempty"`
	Typings          string                 `json:"typings,omitempty"`
	Dependencies     map[string]string      `json:"dependencies,omitempty"`
	PeerDependencies map[string]string      `json:"peerDependencies,omitempty"`
	Imports          map[string]interface{} `json:"imports,omitempty"`
	DefinedExports   interface{}            `json:"exports,omitempty"`
	Deprecated       interface{}            `json:"deprecated,omitempty"`
}

func (a *NpmPackageTemp) ToNpmPackage() *NpmPackage {
	browser := map[string]string{}
	if a.Browser.Value != "" {
		browser["."] = a.Browser.Value
	}
	if a.Browser.Map != nil {
		for k, v := range a.Browser.Map {
			s, isStr := v.(string)
			if isStr {
				browser[k] = s
			} else {
				b, ok := v.(bool)
				if ok && !b {
					browser[k] = ""
				}
			}
		}
	}
	deprecated := ""
	if a.Deprecated != nil {
		if s, ok := a.Deprecated.(string); ok {
			deprecated = s
		}
	}
	return &NpmPackage{
		Name:             a.Name,
		Version:          a.Version,
		Type:             a.Type,
		Main:             a.Main,
		Module:           a.Module.MainValue(),
		ES2015:           a.ES2015.MainValue(),
		JsNextMain:       a.JsNextMain,
		Types:            a.Types,
		Typings:          a.Typings,
		Browser:          browser,
		Dependencies:     a.Dependencies,
		PeerDependencies: a.PeerDependencies,
		Imports:          a.Imports,
		DefinedExports:   a.DefinedExports,
		Deprecated:       deprecated,
	}
}

// NpmPackage defines the package.json of npm
type NpmPackage struct {
	Name             string
	Version          string
	Type             string
	Main             string
	Module           string
	ES2015           string
	JsNextMain       string
	Types            string
	Typings          string
	Browser          map[string]string
	Dependencies     map[string]string
	PeerDependencies map[string]string
	Imports          map[string]interface{}
	DefinedExports   interface{}
	Deprecated       string
}

func (a *NpmPackage) UnmarshalJSON(b []byte) error {
	var n NpmPackageTemp
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*a = *n.ToNpmPackage()
	return nil
}

func checkNodejs(installDir string) (nodeVer string, yarnVer string, err error) {
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

CheckYarn:
	output, err := exec.Command("yarn", "-v").CombinedOutput()
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
		yarnVer = strings.TrimSpace(string(output))
	}
	return
}

func getPackageInfo(wd string, name string, version string) (info NpmPackage, fromPackageJSON bool, err error) {
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
	a := strings.Split(strings.Trim(name, "/"), "/")
	name = a[0]
	if strings.HasPrefix(name, "@") && len(a) > 1 {
		name = a[0] + "/" + a[1]
	}

	if strings.HasPrefix(version, "=") || strings.HasPrefix(version, "v") {
		version = version[1:]
	}
	if version == "" {
		version = "latest"
	}

	cacheKey := fmt.Sprintf("npm:%s@%s", name, version)

	mutex, loaded := lock.LoadOrStore(cacheKey, &sync.Mutex{})

	log.Debugf("fetch lock: %s, loaded(%v)", cacheKey, loaded)
	mutex.(*sync.Mutex).Lock()
	defer func() {
		log.Debugf("fetch  unlock: %s", cacheKey)
		mutex.(*sync.Mutex).Unlock()
	}()

	// check cache firstly
	if cache != nil {
		var data []byte
		data, err = cache.Get(cacheKey)
		if err == nil && json.Unmarshal(data, &info) == nil {
			return
		}
		if err != nil && err != storage.ErrNotFound && err != storage.ErrExpired {
			log.Error("cache:", err)
		}
	}

	start := time.Now()
	req, err := http.NewRequest("GET", strings.TrimRight(cfg.NpmRegistry, "/")+"/"+name, nil)
	if err != nil {
		return
	}
	if cfg.NpmToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.NpmToken)
	}

	resp, err := httpClient.Do(req)
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
		err = fmt.Errorf("npm: could not get metadata of package '%s' (%s: %s)", name, resp.Status, string(ret))
		return
	}

	var h NpmPackageVerions
	err = json.NewDecoder(resp.Body).Decode(&h)
	if err != nil {
		return
	}

	if len(h.Versions) == 0 {
		err = fmt.Errorf("npm: versions of %s not found", name)
		return
	}

	isFullVersion := regexpFullVersion.MatchString(version)
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

	// cache package info for 10 minutes
	if cache != nil {
		var ttl time.Duration = 0
		if !isFullVersion {
			ttl = 10 * time.Minute
		}
		cache.Set(cacheKey, utils.MustEncodeJSON(info), ttl)
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

// see https://nodejs.org/api/packages.html
func resolvePackageExports(p *NpmPackage, conditions interface{}, target string, isDev bool, pType string) {
	s, ok := conditions.(string)
	if ok {
		if pType == "module" {
			p.Module = s
		} else {
			p.Main = s
		}
		return
	}

	m, ok := conditions.(map[string]interface{})
	if ok {
		targetDeno := target == "deno" || target == "denonext"
		names := []string{"browser", "module", "import", "es2015", "worker"}
		if targetDeno {
			names = []string{"deno", "worker", "module", "import", "es2015", "browser"}
		}
		if pType == "module" {
			if isDev {
				names = append([]string{"development"}, names...)
			}
			names = append(names, "default")
		}
		// support solid.js (<=1.6) for deno target
		if (p.Name == "solid-js" || strings.HasPrefix(p.Name, "solid-js/")) && targetDeno {
			names = append([]string{"deno", "worker", "node"}, names...)
		}
		for _, name := range names {
			value, ok := m[name]
			if ok {
				resolvePackageExports(p, value, target, isDev, "module")
				break
			}
		}
		if p.Module == "" {
			for _, name := range []string{"require", "node", "default"} {
				value, ok := m[name]
				if ok {
					resolvePackageExports(p, value, target, isDev, "")
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

var addingLock sync.Map

func yarnAdd(wd string, pkg Pkg) (err error) {
	noCache := false
	pkgNameFormat := fmt.Sprintf("%s@%s", pkg.Name, pkg.Version)

	mutex, loaded := addingLock.LoadOrStore(pkgNameFormat, &sync.Mutex{})

	log.Debugf("yarn add lock: %s, loaded(%v)", pkgNameFormat, loaded)
	mutex.(*sync.Mutex).Lock()
	defer func() {
		log.Debugf("yarn add unlock: %s", pkgNameFormat)
		mutex.(*sync.Mutex).Unlock()
	}()

	// file exist, skip install
	if _, e := os.Stat(path.Join(wd, ".yarn_added")); e == nil {
		return
	}

	for i := 0; i < 3; i++ {
		err = runYarnAdd(wd, noCache, pkgNameFormat)
		if err == nil && !fileExists(path.Join(wd, "node_modules", pkg.Name, "package.json")) {
			noCache = true
			err = fmt.Errorf("yarnAdd(%s): package.json not found", pkg)
		}
		if err == nil {
			break
		}
		if i < 2 {
			time.Sleep(100 * time.Millisecond)
		}
	}
	return
}

func runYarnAdd(wd string, noCache bool, packages ...string) (err error) {
	if len(packages) > 0 {
		start := time.Now()
		args := []string{
			"add",
			"--check-files",
			"--ignore-engines",
			"--ignore-platform",
			"--ignore-scripts",
			"--no-bin-links",
			"--no-node-version-check",
			"--no-progress",
			"--non-interactive",
			"--silent",
			"--registry=" + cfg.NpmRegistry,
		}
		if noCache {
			args = append(args, "--cache-folder", path.Join(wd, ".yarn_cache"))
		} else {
			yarnCacheDir := os.Getenv("YARN_CACHE_DIR")
			if yarnCacheDir != "" {
				args = append(args, "--cache-folder", yarnCacheDir)
			}
		}
		yarnMutex := os.Getenv("YARN_MUTEX")
		if yarnMutex != "" {
			args = append(args, "--mutex", yarnMutex)
		}
		cmd := exec.Command("yarn", append(args, packages...)...)
		cmd.Dir = wd
		if cfg.NpmToken != "" {
			cmd.Env = append(os.Environ(), "ESM_NPM_TOKEN="+cfg.NpmToken)
		}
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("yarn add %s: %s", strings.Join(packages, ","), string(output))
		}

		os.Create(path.Join(wd, ".yarn_added"))
		log.Debug("yarn add", strings.Join(packages, ","), "in", time.Since(start))
	}
	return
}

// ref https://github.com/npm/validate-npm-package-name
func validatePackageName(name string) bool {
	scope := ""
	nameWithoutScope := name
	if strings.HasPrefix(name, "@") {
		scope, nameWithoutScope = utils.SplitByFirstByte(name, '/')
		scope = scope[1:]
	}
	if (scope != "" && !npmNaming.Is(scope)) || (nameWithoutScope == "" || !npmNaming.Is(nameWithoutScope)) || len(name) > 214 {
		return false
	}
	return true
}

// added by @jimisaacs
func toTypesPackageName(pkgName string) string {
	if strings.HasPrefix(pkgName, "@") {
		pkgName = strings.Replace(pkgName[1:], "/", "__", 1)
	}
	return "@types/" + pkgName
}

func fixPkgVersion(info NpmPackage) (NpmPackage, error) {
	for prefix, ver := range fixedPkgVersions {
		if strings.HasPrefix(info.Name+"@"+info.Version, prefix) {
			return fetchPackageInfo(info.Name, ver)
		}
	}
	return info, nil
}

func isTypesOnlyPackage(p NpmPackage) bool {
	return p.Main == "" && p.Module == "" && p.Types != ""
}

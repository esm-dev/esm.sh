package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"sort"
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

// NpmPackageVerions defines versions of a NPM package
type NpmPackageVerions struct {
	DistTags map[string]string     `json:"dist-tags"`
	Versions map[string]NpmPackage `json:"versions"`
}

// NpmPackageTemp defines the package.json of NPM
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
	SideEffects      interface{}            `json:"sideEffects,omitempty"`
	Dependencies     map[string]string      `json:"dependencies,omitempty"`
	PeerDependencies map[string]string      `json:"peerDependencies,omitempty"`
	Imports          map[string]interface{} `json:"imports,omitempty"`
	TypesVersions    map[string]interface{} `json:"typesVersions,omitempty"`
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
	sideEffects := true
	if a.SideEffects != nil {
		if s, ok := a.SideEffects.(string); ok {
			sideEffects = s != "false"
		} else if b, ok := a.SideEffects.(bool); ok {
			sideEffects = b
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
		SideEffects:      sideEffects,
		Dependencies:     a.Dependencies,
		PeerDependencies: a.PeerDependencies,
		Imports:          a.Imports,
		TypesVersions:    a.TypesVersions,
		DefinedExports:   a.DefinedExports,
		Deprecated:       deprecated,
	}
}

// NpmPackage defines the package.json
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
	SideEffects      bool
	Browser          map[string]string
	Dependencies     map[string]string
	PeerDependencies map[string]string
	Imports          map[string]interface{}
	TypesVersions    map[string]interface{}
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
			fromPackageJSON = true
			return
		}
	}

	info, err = fetchPackageInfo(name, version)
	if err == nil {
		info, err = fixPkgVersion(info)
	}
	return
}

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
	isFullVersion := regexpFullVersion.MatchString(version)

	cacheKey := fmt.Sprintf("npm:%s@%s", name, version)
	lock := getFetchLock(cacheKey)
	lock.Lock()
	defer lock.Unlock()

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
	defer func() {
		if err == nil {
			log.Debugf("lookup package(%s@%s) in %v", name, info.Version, time.Since(start))
		}
	}()

	url := cfg.NpmRegistry + name
	if cfg.NpmRegistryScope != "" {
		isInScope := strings.HasPrefix(name, cfg.NpmRegistryScope)
		if !isInScope {
			url = "https://registry.npmjs.org/" + name
		}
	}

	if isFullVersion {
		url += "/" + version
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return
	}
	if cfg.NpmToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.NpmToken)
	}
	if cfg.NpmUser != "" && cfg.NpmPassword != "" {
		req.SetBasicAuth(cfg.NpmUser, cfg.NpmPassword)
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

	if isFullVersion {
		err = json.NewDecoder(resp.Body).Decode(&info)
		if err != nil {
			return
		}
		if cache != nil {
			cache.Set(cacheKey, utils.MustEncodeJSON(info), 24*time.Hour)
		}
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

	if info.Version == "" {
		err = fmt.Errorf("npm: version '%s' of %s not found", version, name)
		return
	}

	// cache package info for 10 minutes
	if cache != nil {
		cache.Set(cacheKey, utils.MustEncodeJSON(info), 10*time.Minute)
	}
	return
}

func installPackage(wd string, pkg Pkg) (err error) {
	pkgVersionName := pkg.VersionName()
	lock := getInstallLock(pkgVersionName)
	lock.Lock()
	defer lock.Unlock()

	// ensure package.json file to prevent read up-levels
	packageFilePath := path.Join(wd, "package.json")
	if pkg.FromEsmsh {
		err = copyRawBuildFile(pkg.Name, "package.json", wd)
	} else if pkg.FromGithub || !fileExists(packageFilePath) {
		fileContent := []byte("{}")
		if pkg.FromGithub {
			fileContent = []byte(fmt.Sprintf(
				`{"dependencies": {"%s": "%s"}}`,
				pkg.Name,
				fmt.Sprintf("git+https://github.com/%s.git#%s", pkg.Name, pkg.Version),
			))
		}
		ensureDir(wd)
		err = os.WriteFile(packageFilePath, fileContent, 0644)
	}
	if err != nil {
		return fmt.Errorf("ensure package.json failed: %s", pkgVersionName)
	}

	for i := 0; i < 3; i++ {
		if pkg.FromEsmsh {
			err = pnpmInstall(wd)
			if err == nil {
				installDir := path.Join(wd, "node_modules", pkg.Name)
				for _, name := range []string{"package.json", "index.mjs", "index.d.ts"} {
					err = copyRawBuildFile(pkg.Name, name, installDir)
					if err != nil {
						break
					}
				}
			}
		} else if pkg.FromGithub {
			err = pnpmInstall(wd)
			// pnpm will ignore github package which has been installed without `package.json` file
			if err == nil && !dirExists(path.Join(wd, "node_modules", pkg.Name)) {
				err = ghInstall(wd, pkg.Name, pkg.Version)
			}
		} else if regexpFullVersion.MatchString(pkg.Version) {
			err = pnpmInstall(wd, pkgVersionName, "--prefer-offline")
		} else {
			err = pnpmInstall(wd, pkgVersionName)
		}
		packageFilePath := path.Join(wd, "node_modules", pkg.Name, "package.json")
		if err == nil && !fileExists(packageFilePath) {
			if pkg.FromGithub {
				ensureDir(path.Dir(packageFilePath))
				err = ioutil.WriteFile(packageFilePath, utils.MustEncodeJSON(pkg), 0644)
			} else {
				err = fmt.Errorf("pnpm install %s: package.json not found", pkg)
			}
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

func pnpmInstall(wd string, packages ...string) (err error) {
	var args []string
	if len(packages) > 0 {
		args = append([]string{"add"}, packages...)
	} else {
		args = []string{"install"}
	}
	args = append(
		args,
		"--ignore-scripts",
		"--loglevel", "error",
	)
	start := time.Now()
	cmd := exec.Command("pnpm", args...)
	cmd.Dir = wd
	if cfg.NpmToken != "" {
		cmd.Env = append(os.Environ(), "ESM_NPM_TOKEN="+cfg.NpmToken)
	}
	if cfg.NpmUser != "" && cfg.NpmPassword != "" {
		data := []byte(cfg.NpmPassword)
		password := make([]byte, base64.StdEncoding.EncodedLen(len(data)))
		base64.StdEncoding.Encode(password, data)
		cmd.Env = append(
			os.Environ(),
			"ESM_NPM_USER="+cfg.NpmUser,
			"ESM_NPM_PASSWORD="+string(password),
		)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pnpm add %s: %s", strings.Join(packages, ","), string(output))
	}
	if len(packages) > 0 {
		log.Debug("pnpm add", strings.Join(packages, ","), "in", time.Since(start))
	} else {
		log.Debug("pnpm install in", time.Since(start))
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

func getInstallLock(key string) *sync.Mutex {
	v, _ := installLocks.LoadOrStore(key, &sync.Mutex{})
	return v.(*sync.Mutex)
}

func getFetchLock(key string) *sync.Mutex {
	v, _ := fetchLocks.LoadOrStore(key, &sync.Mutex{})
	return v.(*sync.Mutex)
}

package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/esm-dev/esm.sh/server/common"
	"github.com/ije/gox/utils"
	"github.com/ije/gox/valid"
)

const (
	npmRegistry = "https://registry.npmjs.org/"
	jsrRegistry = "https://npm.jsr.io/"
)

var (
	installLocks = sync.Map{}
	npmNaming    = valid.Validator{valid.Range{'a', 'z'}, valid.Range{'A', 'Z'}, valid.Range{'0', '9'}, valid.Eq('_'), valid.Eq('$'), valid.Eq('.'), valid.Eq('-'), valid.Eq('+'), valid.Eq('!'), valid.Eq('~'), valid.Eq('*'), valid.Eq('('), valid.Eq(')')}
)

type Package struct {
	Github   bool
	PkgPrNew bool
	Name     string
	Version  string
}

func (p *Package) FullName() string {
	if p.Github {
		return "gh/" + p.Name
	}
	if p.PkgPrNew {
		return "pr/" + p.Name
	}
	return p.Name
}

func (p *Package) String() string {
	s := p.Name + "@" + p.Version
	if p.Github {
		return "gh/" + s
	}
	if p.PkgPrNew {
		return "pr/" + s
	}
	return s
}

// NpmPackageVerions defines versions of a NPM package
type NpmPackageVerions struct {
	DistTags map[string]string         `json:"dist-tags"`
	Versions map[string]PackageJSONRaw `json:"versions"`
}

// NpmDist defines the dist field of a NPM package
type NpmDist struct {
	Tarball string `json:"tarball"`
}

// PackageJSONRaw defines the package.json of a NPM package
type PackageJSONRaw struct {
	Name             string          `json:"name"`
	Version          string          `json:"version"`
	Type             string          `json:"type"`
	Main             JsonAny         `json:"main"`
	Module           JsonAny         `json:"module"`
	ES2015           JsonAny         `json:"es2015"`
	JsNextMain       JsonAny         `json:"jsnext:main"`
	Browser          JsonAny         `json:"browser"`
	Types            JsonAny         `json:"types"`
	Typings          JsonAny         `json:"typings"`
	SideEffects      any             `json:"sideEffects"`
	Dependencies     any             `json:"dependencies"`
	PeerDependencies any             `json:"peerDependencies"`
	Imports          any             `json:"imports"`
	TypesVersions    any             `json:"typesVersions"`
	Exports          json.RawMessage `json:"exports"`
	Files            []string        `json:"files"`
	Esmsh            any             `json:"esm.sh"`
	Dist             json.RawMessage `json:"dist"`
	Deprecated       any             `json:"deprecated"`
}

// PackageJSON defines the package.json of a NPM package
type PackageJSON struct {
	Name             string
	PkgName          string
	Version          string
	Type             string
	Main             string
	Module           string
	Types            string
	Typings          string
	SideEffectsFalse bool
	SideEffects      *StringSet
	Browser          map[string]string
	Dependencies     map[string]string
	PeerDependencies map[string]string
	Imports          map[string]any
	TypesVersions    map[string]any
	Exports          *OrderedMap
	Esmsh            map[string]any
	Dist             NpmDist
	Deprecated       string
}

// ToNpmPackage converts PackageJSONRaw to PackageJSON
func (a *PackageJSONRaw) ToNpmPackage() *PackageJSON {
	browser := map[string]string{}
	if a.Browser.Str != "" {
		browser["."] = a.Browser.Str
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
	var dependencies map[string]string
	if m, ok := a.Dependencies.(map[string]any); ok {
		dependencies = make(map[string]string, len(m))
		for k, v := range m {
			if s, ok := v.(string); ok {
				if k != "" && s != "" {
					dependencies[k] = s
				}
			}
		}
	}
	var peerDependencies map[string]string
	if m, ok := a.PeerDependencies.(map[string]any); ok {
		peerDependencies = make(map[string]string, len(m))
		for k, v := range m {
			if s, ok := v.(string); ok {
				if k != "" && s != "" {
					peerDependencies[k] = s
				}
			}
		}
	}
	var sideEffects *StringSet = nil
	sideEffectsFalse := false
	if a.SideEffects != nil {
		if s, ok := a.SideEffects.(string); ok {
			if s == "false" {
				sideEffectsFalse = true
			} else if endsWith(s, moduleExts...) {
				sideEffects = NewStringSet()
				sideEffects.Add(s)
			}
		} else if b, ok := a.SideEffects.(bool); ok {
			sideEffectsFalse = !b
		} else if m, ok := a.SideEffects.([]any); ok && len(m) > 0 {
			sideEffects = NewStringSet()
			for _, v := range m {
				if name, ok := v.(string); ok && endsWith(name, moduleExts...) {
					sideEffects.Add(name)
				}
			}
		}
	}
	exports := newOrderedMap()
	if rawExports := a.Exports; rawExports != nil {
		var s string
		if json.Unmarshal(rawExports, &s) == nil {
			if len(s) > 0 {
				exports.Set(".", s)
			}
		} else {
			exports.UnmarshalJSON(rawExports)
		}
	}
	depreacted := ""
	if a.Deprecated != nil {
		if s, ok := a.Deprecated.(string); ok {
			depreacted = s
		}
	}
	var dist NpmDist
	if a.Dist != nil {
		json.Unmarshal(a.Dist, &dist)
	}

	p := &PackageJSON{
		Name:             a.Name,
		Version:          a.Version,
		Type:             a.Type,
		Main:             a.Main.String(),
		Module:           a.Module.String(),
		Types:            a.Types.String(),
		Typings:          a.Typings.String(),
		Browser:          browser,
		SideEffectsFalse: sideEffectsFalse,
		SideEffects:      sideEffects,
		Dependencies:     dependencies,
		PeerDependencies: peerDependencies,
		Imports:          toMap(a.Imports),
		TypesVersions:    toMap(a.TypesVersions),
		Exports:          exports,
		Esmsh:            toMap(a.Esmsh),
		Deprecated:       depreacted,
		Dist:             dist,
	}

	// normalize package module field
	if p.Module == "" {
		if es2015 := a.ES2015.String(); es2015 != "" {
			p.Module = es2015
		} else if jsNextMain := a.JsNextMain.String(); jsNextMain != "" {
			p.Module = jsNextMain
		} else if p.Main != "" && (p.Type == "module" || strings.HasSuffix(p.Main, ".mjs")) {
			p.Module = p.Main
			p.Main = ""
		}
	}

	return p
}

// UnmarshalJSON implements the json.Unmarshaler interface
func (a *PackageJSON) UnmarshalJSON(b []byte) error {
	var n PackageJSONRaw
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*a = *n.ToNpmPackage()
	return nil
}

type NpmRegistry struct {
	Registry string `json:"registry"`
	Token    string `json:"token"`
	User     string `json:"user"`
	Password string `json:"password"`
}

type NpmRC struct {
	NpmRegistry
	ScopedRegistries map[string]NpmRegistry `json:"scopedRegistries"`
	zoneId           string
}

var (
	defaultNpmRC *NpmRC
)

func getDefaultNpmRC() *NpmRC {
	if defaultNpmRC != nil {
		return defaultNpmRC
	}
	defaultNpmRC = &NpmRC{
		NpmRegistry: NpmRegistry{
			Registry: config.NpmRegistry,
			Token:    config.NpmToken,
			User:     config.NpmUser,
			Password: config.NpmPassword,
		},
		ScopedRegistries: map[string]NpmRegistry{
			"@jsr": {
				Registry: jsrRegistry,
			},
		},
	}
	if len(config.NpmScopedRegistries) > 0 {
		for scope, reg := range config.NpmScopedRegistries {
			defaultNpmRC.ScopedRegistries[scope] = NpmRegistry{
				Registry: reg.Registry,
				Token:    reg.Token,
				User:     reg.User,
				Password: reg.Password,
			}
		}
	}
	return defaultNpmRC
}

func NewNpmRcFromJSON(jsonData []byte) (npmrc *NpmRC, err error) {
	var rc NpmRC
	err = json.Unmarshal(jsonData, &rc)
	if err != nil {
		return nil, err
	}
	if rc.Registry == "" {
		rc.Registry = config.NpmRegistry
	} else if !strings.HasSuffix(rc.Registry, "/") {
		rc.Registry += "/"
	}
	if rc.ScopedRegistries == nil {
		rc.ScopedRegistries = map[string]NpmRegistry{}
	}
	if _, ok := rc.ScopedRegistries["@jsr"]; !ok {
		rc.ScopedRegistries["@jsr"] = NpmRegistry{
			Registry: jsrRegistry,
		}
	}
	for _, reg := range rc.ScopedRegistries {
		if reg.Registry != "" && !strings.HasSuffix(reg.Registry, "/") {
			reg.Registry += "/"
		}
	}
	return &rc, nil
}

func (rc *NpmRC) StoreDir() string {
	if rc.zoneId != "" {
		return path.Join(config.WorkDir, "npm-"+rc.zoneId)
	}
	return path.Join(config.WorkDir, "npm")
}

func (rc *NpmRC) getPackageInfo(name string, semver string) (info *PackageJSON, err error) {
	// use fixed version for `@types/node`
	if name == "@types/node" {
		info = &PackageJSON{
			Name:    "@types/node",
			Version: nodeTypesVersion,
			Types:   "index.d.ts",
		}
		return
	}

	// strip leading `=` or `v`
	if (strings.HasPrefix(semver, "=") || strings.HasPrefix(semver, "v")) && regexpVersionStrict.MatchString(semver[1:]) {
		semver = semver[1:]
	}

	// check if the package has been installed
	if regexpVersionStrict.MatchString(semver) {
		var raw PackageJSONRaw
		pkgJsonPath := path.Join(rc.StoreDir(), name+"@"+semver, "node_modules", name, "package.json")
		if existsFile(pkgJsonPath) && utils.ParseJSONFile(pkgJsonPath, &raw) == nil {
			info = raw.ToNpmPackage()
			return
		}
	}

	info, err = rc.fetchPackageInfo(name, semver)
	return
}

func (npmrc *NpmRC) getRegistryByPackageName(packageName string) *NpmRegistry {
	if strings.HasPrefix(packageName, "@") {
		scope, _ := utils.SplitByFirstByte(packageName, '/')
		reg, ok := npmrc.ScopedRegistries[scope]
		if ok {
			return &reg
		}
	}
	return &npmrc.NpmRegistry
}

func (npmrc *NpmRC) fetchPackageInfo(packageName string, semverOrDistTag string) (packageJson *PackageJSON, err error) {
	a := strings.Split(strings.Trim(packageName, "/"), "/")
	packageName = a[0]
	if strings.HasPrefix(packageName, "@") && len(a) > 1 {
		packageName = a[0] + "/" + a[1]
	}

	if semverOrDistTag == "" || semverOrDistTag == "*" {
		semverOrDistTag = "latest"
	} else if (strings.HasPrefix(semverOrDistTag, "=") || strings.HasPrefix(semverOrDistTag, "v")) && regexpVersionStrict.MatchString(semverOrDistTag[1:]) {
		// strip leading `=` or `v` from semver
		semverOrDistTag = semverOrDistTag[1:]
	}

	reg := npmrc.getRegistryByPackageName(packageName)
	genCacheKey := func(packageName string, packageVersion string) string {
		return reg.Registry + packageName + "@" + packageVersion + "?auth=" + reg.Token + "|" + reg.User + ":" + reg.Password
	}
	cacheKey := genCacheKey(packageName, semverOrDistTag)

	return withCache(cacheKey, time.Duration(config.NpmQueryCacheTTL)*time.Second, func() (*PackageJSON, string, error) {
		url := reg.Registry + packageName
		isFullVersion := regexpVersionStrict.MatchString(semverOrDistTag)
		isFullVersionFromNpmjsOrg := isFullVersion && strings.HasPrefix(url, npmRegistry)
		if isFullVersionFromNpmjsOrg {
			// npm registry supports url like `https://registry.npmjs.org/<name>/<version>`
			url += "/" + semverOrDistTag
		}

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, "", err
		}

		if reg.Token != "" {
			req.Header.Set("Authorization", "Bearer "+reg.Token)
		} else if reg.User != "" && reg.Password != "" {
			req.SetBasicAuth(reg.User, reg.Password)
		}

		c := &http.Client{
			Timeout: 15 * time.Second,
		}
		retryTimes := 0
	do:
		res, err := c.Do(req)
		if err != nil {
			if retryTimes < 3 {
				retryTimes++
				time.Sleep(time.Duration(retryTimes) * 100 * time.Millisecond)
				goto do
			}
			return nil, "", err
		}
		defer res.Body.Close()

		if res.StatusCode == 404 || res.StatusCode == 401 {
			if isFullVersionFromNpmjsOrg {
				err = fmt.Errorf("version %s of '%s' not found", semverOrDistTag, packageName)
			} else {
				err = fmt.Errorf("package '%s' not found", packageName)
			}
			return nil, "", err
		}

		if res.StatusCode != 200 {
			msg, _ := io.ReadAll(res.Body)
			return nil, "", fmt.Errorf("could not get metadata of package '%s' (%s: %s)", packageName, res.Status, string(msg))
		}

		if isFullVersionFromNpmjsOrg {
			var raw PackageJSONRaw
			err = json.NewDecoder(res.Body).Decode(&raw)
			if err != nil {
				return nil, "", err
			}
			return raw.ToNpmPackage(), genCacheKey(packageName, raw.Version), nil
		}

		var h NpmPackageVerions
		err = json.NewDecoder(res.Body).Decode(&h)
		if err != nil {
			return nil, "", err
		}

		if len(h.Versions) == 0 {
			return nil, "", fmt.Errorf("version %s of '%s' not found", semverOrDistTag, packageName)
		}

	lookup:
		distVersion, ok := h.DistTags[semverOrDistTag]
		if ok {
			raw, ok := h.Versions[distVersion]
			if ok {
				return raw.ToNpmPackage(), genCacheKey(packageName, raw.Version), nil
			}
		} else {
			if semverOrDistTag == "lastest" {
				return nil, "", fmt.Errorf("version %s of '%s' not found", semverOrDistTag, packageName)
			}
			var c *semver.Constraints
			c, err = semver.NewConstraint(semverOrDistTag)
			if err != nil {
				semverOrDistTag = "latest"
				goto lookup
			}
			vs := make([]*semver.Version, len(h.Versions))
			i := 0
			for v := range h.Versions {
				// ignore prerelease versions
				if !strings.ContainsRune(semverOrDistTag, '-') && strings.ContainsRune(v, '-') {
					continue
				}
				var ver *semver.Version
				ver, err = semver.NewVersion(v)
				if err != nil {
					return nil, "", err
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
				raw, ok := h.Versions[vs[i-1].String()]
				if ok {
					return raw.ToNpmPackage(), genCacheKey(packageName, raw.Version), nil
				}
			}
		}
		return nil, "", fmt.Errorf("version %s of '%s' not found", semverOrDistTag, packageName)
	})
}

func (rc *NpmRC) installPackage(pkg Package) (packageJson *PackageJSON, err error) {
	installDir := path.Join(rc.StoreDir(), pkg.String())
	packageJsonPath := path.Join(installDir, "node_modules", pkg.Name, "package.json")

	// check if the package has been installed
	if existsFile(packageJsonPath) {
		var raw PackageJSONRaw
		if utils.ParseJSONFile(packageJsonPath, &raw) == nil {
			packageJson = raw.ToNpmPackage()
			return
		}
	}

	// only one installation process is allowed at the same time for the same package
	v, _ := installLocks.LoadOrStore(pkg.FullName(), &sync.Mutex{})
	defer installLocks.Delete(pkg.FullName())

	v.(*sync.Mutex).Lock()
	defer v.(*sync.Mutex).Unlock()

	// skip installation if the package has been installed
	if existsFile(packageJsonPath) {
		var raw PackageJSONRaw
		if utils.ParseJSONFile(packageJsonPath, &raw) == nil {
			packageJson = raw.ToNpmPackage()
			return
		}
	}

	err = ensureDir(installDir)
	if err != nil {
		return
	}

	if pkg.Github {
		err = ghInstall(installDir, pkg.Name, pkg.Version)
		// ensure 'package.json' file if not exists after installing from github
		if err == nil && !existsFile(packageJsonPath) {
			buf := bytes.NewBuffer(nil)
			fmt.Fprintf(buf, `{"name":"%s","version":"%s"}`, pkg.Name, pkg.Version)
			err = os.WriteFile(packageJsonPath, buf.Bytes(), 0644)
		}
	} else if pkg.PkgPrNew {
		err = rc.downloadTarball(&NpmRegistry{}, installDir, pkg.Name, "https://pkg.pr.new/"+pkg.Name+"@"+pkg.Version)
	} else {
		var info *PackageJSON
		info, err = rc.fetchPackageInfo(pkg.Name, pkg.Version)
		if err != nil {
			return nil, err
		}
		if info.Deprecated != "" {
			os.WriteFile(path.Join(installDir, "deprecated.txt"), []byte(info.Deprecated), 0644)
		}
		err = rc.downloadTarball(rc.getRegistryByPackageName(pkg.Name), installDir, info.Name, info.Dist.Tarball)
	}
	if err != nil {
		return
	}

	var raw PackageJSONRaw
	err = utils.ParseJSONFile(packageJsonPath, &raw)
	if err != nil {
		err = fmt.Errorf("failed to install %s: %v", pkg.String(), err)
		return
	}

	packageJson = raw.ToNpmPackage()
	return
}

func (rc *NpmRC) downloadTarball(reg *NpmRegistry, installDir string, pkgName string, tarballUrl string) (err error) {
	req, err := http.NewRequest("GET", tarballUrl, nil)
	if err != nil {
		return
	}

	if reg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+reg.Token)
	} else if reg.User != "" && reg.Password != "" {
		req.SetBasicAuth(reg.User, reg.Password)
	}

	c := &http.Client{
		Timeout: 15 * time.Second,
	}
	retryTimes := 0
do:
	res, err := c.Do(req)
	if err != nil {
		if retryTimes < 3 {
			retryTimes++
			time.Sleep(time.Duration(retryTimes) * 100 * time.Millisecond)
			goto do
		}
		return
	}
	defer res.Body.Close()

	if res.StatusCode == 404 || res.StatusCode == 401 {
		err = fmt.Errorf("tarball of package '%s' not found", path.Base(installDir))
		return
	}

	if res.StatusCode != 200 {
		msg, _ := io.ReadAll(res.Body)
		err = fmt.Errorf("could not download tarball of package '%s' (%s: %s)", path.Base(installDir), res.Status, string(msg))
		return
	}

	err = extractPackageTarball(installDir, pkgName, io.LimitReader(res.Body, 256*MB))
	return
}

func (npmrc *NpmRC) isDeprecated(pkgName string, pkgVersion string) (string, error) {
	installDir := path.Join(npmrc.StoreDir(), pkgName+"@"+pkgVersion)
	data, err := os.ReadFile(path.Join(installDir, "deprecated.txt"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

func (npmrc *NpmRC) getSvelteVersion(importMap common.ImportMap) (svelteVersion string, err error) {
	svelteVersion = "5"
	if len(importMap.Imports) > 0 {
		sveltePath, ok := importMap.Imports["svelte"]
		if ok {
			a := regexpSveltePath.FindAllStringSubmatch(sveltePath, 1)
			if len(a) > 0 {
				svelteVersion = a[0][1]
			}
		}
	}
	if !regexpVersionStrict.MatchString(svelteVersion) {
		var info *PackageJSON
		info, err = npmrc.getPackageInfo("svelte", svelteVersion)
		if err != nil {
			return
		}
		svelteVersion = info.Version
	}
	if semverLessThan(svelteVersion, "4.0.0") {
		err = errors.New("unsupported svelte version, only 4.0.0+ is supported")
	}
	return
}

func (npmrc *NpmRC) getVueVersion(importMap common.ImportMap) (vueVersion string, err error) {
	vueVersion = "3"
	if len(importMap.Imports) > 0 {
		vuePath, ok := importMap.Imports["vue"]
		if ok {
			a := regexpVuePath.FindAllStringSubmatch(vuePath, 1)
			if len(a) > 0 {
				vueVersion = a[0][1]
			}
		}
	}
	if !regexpVersionStrict.MatchString(vueVersion) {
		var info *PackageJSON
		info, err = npmrc.getPackageInfo("vue", vueVersion)
		if err != nil {
			return
		}
		vueVersion = info.Version
	}
	if semverLessThan(vueVersion, "3.0.0") {
		err = errors.New("unsupported vue version, only 3.0.0+ is supported")
	}
	return
}

func resolveDependencyVersion(v string) (Package, error) {
	// ban file specifier
	if strings.HasPrefix(v, "file:") {
		return Package{}, errors.New("unsupported file dependency")
	}
	if strings.HasPrefix(v, "npm:") {
		pkgName, pkgVersion, _, _ := splitEsmPath(v[4:])
		if !validatePackageName(pkgName) {
			return Package{}, errors.New("invalid npm dependency")
		}
		return Package{
			Name:    pkgName,
			Version: pkgVersion,
		}, nil
	}
	if strings.HasPrefix(v, "jsr:") {
		pkgName, pkgVersion, _, _ := splitEsmPath(v[4:])
		if !strings.HasPrefix(pkgName, "@") || !strings.ContainsRune(pkgName, '/') {
			return Package{}, errors.New("invalid jsr dependency")
		}
		scope, name := utils.SplitByFirstByte(pkgName, '/')
		return Package{
			Name:    "@jsr/" + scope[1:] + "__" + name,
			Version: pkgVersion,
		}, nil
	}
	if strings.HasPrefix(v, "github:") {
		repo, fragment := utils.SplitByLastByte(strings.TrimPrefix(v, "github:"), '#')
		return Package{
			Github:  true,
			Name:    repo,
			Version: strings.TrimPrefix(url.QueryEscape(fragment), "semver:"),
		}, nil
	}
	if strings.HasPrefix(v, "git+ssh://") || strings.HasPrefix(v, "git+https://") || strings.HasPrefix(v, "git://") {
		gitUrl, e := url.Parse(v)
		if e != nil || gitUrl.Hostname() != "github.com" {
			return Package{}, errors.New("unsupported git dependency")
		}
		repo := strings.TrimSuffix(gitUrl.Path[1:], ".git")
		if gitUrl.Scheme == "git+ssh" {
			repo = gitUrl.Port() + "/" + repo
		}
		return Package{
			Github:  true,
			Name:    repo,
			Version: strings.TrimPrefix(url.QueryEscape(gitUrl.Fragment), "semver:"),
		}, nil
	}
	// https://pkg.pr.new
	if strings.HasPrefix(v, "https://") || strings.HasPrefix(v, "http://") {
		u, e := url.Parse(v)
		if e != nil || u.Host != "pkg.pr.new" {
			return Package{}, errors.New("unsupported http dependency")
		}
		pkgName, rest := utils.SplitByLastByte(u.Path[1:], '@')
		if rest == "" {
			return Package{}, errors.New("unsupported http dependency")
		}
		version, _ := utils.SplitByFirstByte(rest, '/')
		if version == "" || !regexpVersion.MatchString(version) {
			return Package{}, errors.New("unsupported http dependency")
		}
		return Package{
			PkgPrNew: true,
			Name:     pkgName,
			Version:  version,
		}, nil
	}
	// see https://docs.npmjs.com/cli/v10/configuring-npm/package-json#git-urls-as-dependencies
	if !strings.HasPrefix(v, "@") && strings.ContainsRune(v, '/') {
		repo, fragment := utils.SplitByLastByte(v, '#')
		return Package{
			Github:  true,
			Name:    repo,
			Version: strings.TrimPrefix(url.QueryEscape(fragment), "semver:"),
		}, nil
	}
	return Package{}, nil
}

// based on https://github.com/npm/validate-npm-package-name
func validatePackageName(pkgName string) bool {
	if len(pkgName) > 214 {
		return false
	}
	if strings.HasPrefix(pkgName, "@") {
		scope, name := utils.SplitByFirstByte(pkgName, '/')
		return npmNaming.Match(scope[1:]) && npmNaming.Match(name)
	}
	return npmNaming.Match(pkgName)
}

// added by @jimisaacs
func toTypesPackageName(pkgName string) string {
	if strings.HasPrefix(pkgName, "@") {
		pkgName = strings.Replace(pkgName[1:], "/", "__", 1)
	}
	return "@types/" + pkgName
}

// toMap converts any value to a `map[string]any`
func toMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func extractPackageTarball(installDir string, packname string, tarball io.Reader) (err error) {
	unziped, err := gzip.NewReader(tarball)
	if err != nil {
		return
	}

	rootDir := path.Join(installDir, "node_modules", packname)
	defer func() {
		if err != nil {
			// remove the root dir if failed
			os.RemoveAll(rootDir)
		}
	}()

	// extract tarball
	tr := tar.NewReader(unziped)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// strip tarball root dir
		_, name := utils.SplitByFirstByte(h.Name, '/')
		filename := path.Join(rootDir, name)
		if h.Typeflag == tar.TypeDir {
			ensureDir(filename)
			continue
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		extname := path.Ext(filename)
		if !(extname != "" && (assetExts[extname[1:]] || contains(moduleExts, extname) || extname == ".map" || extname == ".css" || extname == ".svelte" || extname == ".vue")) {
			// skip unsupported formats
			continue
		}
		f, err := os.OpenFile(filename, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
		if err != nil && os.IsNotExist(err) {
			os.MkdirAll(path.Dir(filename), 0755)
			f, err = os.OpenFile(filename, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
		}
		if err != nil {
			return err
		}
		_, err = io.Copy(f, tr)
		f.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

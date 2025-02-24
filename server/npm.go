package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
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
	"github.com/ije/gox/set"
	syncx "github.com/ije/gox/sync"
	"github.com/ije/gox/utils"
	"github.com/ije/gox/valid"
)

const (
	npmRegistry = "https://registry.npmjs.org/"
	jsrRegistry = "https://npm.jsr.io/"
)

var (
	npmNaming     = valid.Validator{valid.Range{'a', 'z'}, valid.Range{'A', 'Z'}, valid.Range{'0', '9'}, valid.Eq('_'), valid.Eq('.'), valid.Eq('-'), valid.Eq('+'), valid.Eq('$'), valid.Eq('!')}
	npmVersioning = valid.Validator{valid.Range{'a', 'z'}, valid.Range{'A', 'Z'}, valid.Range{'0', '9'}, valid.Eq('_'), valid.Eq('.'), valid.Eq('-'), valid.Eq('+')}
	installMutex  = syncx.KeyedMutex{}
)

type Package struct {
	Name     string
	Version  string
	Github   bool
	PkgPrNew bool
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

// NpmPackageMetadata defines versions of a NPM package
type NpmPackageMetadata struct {
	DistTags map[string]string         `json:"dist-tags"`
	Versions map[string]PackageJSONRaw `json:"versions"`
}

// PackageJSONRaw defines the package.json of a NPM package
type PackageJSONRaw struct {
	Name             string          `json:"name"`
	Version          string          `json:"version"`
	Type             string          `json:"type"`
	Main             JSONAny         `json:"main"`
	Module           JSONAny         `json:"module"`
	ES2015           JSONAny         `json:"es2015"`
	JsNextMain       JSONAny         `json:"jsnext:main"`
	Browser          JSONAny         `json:"browser"`
	Types            JSONAny         `json:"types"`
	Typings          JSONAny         `json:"typings"`
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

// NpmPackageDist defines the dist field of a NPM package
type NpmPackageDist struct {
	Tarball string `json:"tarball"`
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
	SideEffects      set.ReadOnlySet[string]
	Browser          map[string]string
	Dependencies     map[string]string
	PeerDependencies map[string]string
	Imports          map[string]any
	TypesVersions    map[string]any
	Exports          JSONObject
	Esmsh            map[string]any
	Dist             NpmPackageDist
	Deprecated       string
}

// ToNpmPackage converts PackageJSONRaw to PackageJSON
func (a *PackageJSONRaw) ToNpmPackage() *PackageJSON {
	browser := map[string]string{}
	if a.Browser.Str != "" && endsWith(a.Browser.Str, moduleExts...) {
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
		dependencies = make(map[string]string)
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
		peerDependencies = make(map[string]string)
		for k, v := range m {
			if s, ok := v.(string); ok {
				if k != "" && s != "" {
					peerDependencies[k] = s
				}
			}
		}
	}

	sideEffects := set.New[string]()
	sideEffectsFalse := false
	if a.SideEffects != nil {
		if s, ok := a.SideEffects.(string); ok {
			if s == "false" {
				sideEffectsFalse = true
			} else if endsWith(s, moduleExts...) {
				sideEffects = set.New[string]()
				sideEffects.Add(s)
			}
		} else if b, ok := a.SideEffects.(bool); ok {
			sideEffectsFalse = !b
		} else if m, ok := a.SideEffects.([]any); ok && len(m) > 0 {
			sideEffects = set.New[string]()
			for _, v := range m {
				if name, ok := v.(string); ok && endsWith(name, moduleExts...) {
					sideEffects.Add(name)
				}
			}
		}
	}

	exports := JSONObject{}
	if rawExports := a.Exports; rawExports != nil {
		var s string
		if json.Unmarshal(rawExports, &s) == nil {
			if len(s) > 0 {
				exports = JSONObject{
					keys:   []string{"."},
					values: map[string]any{".": s},
				}
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

	var dist NpmPackageDist
	if a.Dist != nil {
		json.Unmarshal(a.Dist, &dist)
	}

	p := &PackageJSON{
		Name:             a.Name,
		Version:          a.Version,
		Type:             a.Type,
		Main:             a.Main.MainString(),
		Module:           a.Module.MainString(),
		Types:            a.Types.MainString(),
		Typings:          a.Typings.MainString(),
		Browser:          browser,
		SideEffectsFalse: sideEffectsFalse,
		SideEffects:      *sideEffects.ReadOnly(),
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
		if es2015 := a.ES2015.MainString(); es2015 != "" {
			p.Module = es2015
		} else if jsNextMain := a.JsNextMain.MainString(); jsNextMain != "" {
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
	var raw PackageJSONRaw
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*a = *raw.ToNpmPackage()
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

func DefaultNpmRC() *NpmRC {
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

func (npmrc *NpmRC) getPackageInfo(pkgName string, version string) (packageJson *PackageJSON, err error) {
	reg := npmrc.getRegistryByPackageName(pkgName)
	getCacheKey := func(pkgName string, pkgVersion string) string {
		return reg.Registry + pkgName + "@" + pkgVersion
	}

	version = normalizePackageVersion(version)
	return withCache(getCacheKey(pkgName, version), time.Duration(config.NpmQueryCacheTTL)*time.Second, func() (*PackageJSON, string, error) {
		// check if the package has been installed
		if !isDistTag(version) && isExactVersion(version) {
			var raw PackageJSONRaw
			pkgJsonPath := path.Join(npmrc.StoreDir(), pkgName+"@"+version, "node_modules", pkgName, "package.json")
			if utils.ParseJSONFile(pkgJsonPath, &raw) == nil {
				return raw.ToNpmPackage(), "", nil
			}
		}

		regUrl := reg.Registry + pkgName
		isWellknownVersion := (isExactVersion(version) || isDistTag(version)) && strings.HasPrefix(regUrl, npmRegistry)
		if isWellknownVersion {
			// npm registry supports url like `https://registry.npmjs.org/<name>/<version>`
			regUrl += "/" + version
		}

		u, err := url.Parse(regUrl)
		if err != nil {
			return nil, "", err
		}

		header := http.Header{}
		if reg.Token != "" {
			header.Set("Authorization", "Bearer "+reg.Token)
		} else if reg.User != "" && reg.Password != "" {
			header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(reg.User+":"+reg.Password)))
		}

		fetchClient, recycle := NewFetchClient(15, "esmd/"+VERSION, false)
		defer recycle()

		retryTimes := 0
	RETRY:
		res, err := fetchClient.Fetch(u, header)
		if err != nil {
			if retryTimes < 3 {
				retryTimes++
				time.Sleep(time.Duration(retryTimes) * 100 * time.Millisecond)
				goto RETRY
			}
			return nil, "", err
		}
		defer res.Body.Close()

		if res.StatusCode == 404 || res.StatusCode == 401 {
			if isWellknownVersion {
				err = fmt.Errorf("version %s of '%s' not found", version, pkgName)
			} else {
				err = fmt.Errorf("package '%s' not found", pkgName)
			}
			return nil, "", err
		}

		if res.StatusCode != 200 {
			msg, _ := io.ReadAll(res.Body)
			return nil, "", fmt.Errorf("could not get metadata of package '%s' (%s: %s)", pkgName, res.Status, string(msg))
		}

		if isWellknownVersion {
			var raw PackageJSONRaw
			err = json.NewDecoder(res.Body).Decode(&raw)
			if err != nil {
				return nil, "", err
			}
			return raw.ToNpmPackage(), getCacheKey(pkgName, raw.Version), nil
		}

		var metadata NpmPackageMetadata
		err = json.NewDecoder(res.Body).Decode(&metadata)
		if err != nil {
			return nil, "", err
		}

		if len(metadata.Versions) == 0 {
			return nil, "", fmt.Errorf("version %s of '%s' not found", version, pkgName)
		}

	CHECK:
		distVersion, ok := metadata.DistTags[version]
		if ok {
			raw, ok := metadata.Versions[distVersion]
			if ok {
				return raw.ToNpmPackage(), getCacheKey(pkgName, raw.Version), nil
			}
		} else {
			if version == "lastest" {
				return nil, "", fmt.Errorf("version %s of '%s' not found", version, pkgName)
			}
			var c *semver.Constraints
			c, err = semver.NewConstraint(version)
			if err != nil {
				// fallback to latest if semverOrDistTag is not a valid semver
				version = "latest"
				goto CHECK
			}
			vs := make([]*semver.Version, len(metadata.Versions))
			i := 0
			for v := range metadata.Versions {
				// ignore prerelease versions
				if !strings.ContainsRune(version, '-') && strings.ContainsRune(v, '-') {
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
				raw, ok := metadata.Versions[vs[i-1].String()]
				if ok {
					return raw.ToNpmPackage(), getCacheKey(pkgName, raw.Version), nil
				}
			}
		}
		return nil, "", fmt.Errorf("version %s of '%s' not found", version, pkgName)
	})
}

func (npmrc *NpmRC) installPackage(pkg Package) (packageJson *PackageJSON, err error) {
	installDir := path.Join(npmrc.StoreDir(), pkg.String())
	packageJsonPath := path.Join(installDir, "node_modules", pkg.Name, "package.json")

	// check if the package has been installed
	var raw PackageJSONRaw
	if utils.ParseJSONFile(packageJsonPath, &raw) == nil {
		packageJson = raw.ToNpmPackage()
		return
	}

	// only one installation process is allowed at the same time for the same package
	unlock := installMutex.Lock(pkg.String())
	defer unlock()

	// skip installation if the package has been installed by another request
	if utils.ParseJSONFile(packageJsonPath, &raw) == nil {
		packageJson = raw.ToNpmPackage()
		return
	}

	if pkg.Github {
		err = ghInstall(installDir, pkg.Name, pkg.Version)
		// ensure 'package.json' file if not exists after installing from github
		if err == nil && !existsFile(packageJsonPath) {
			buf := bytes.NewBuffer(nil)
			buf.WriteString(`{"name":"` + pkg.Name + `","version":"` + pkg.Version + `"`)
			var denoJson *PackageJSON
			if deonJsonPath := path.Join(installDir, "node_modules", pkg.Name, "deno.json"); existsFile(deonJsonPath) {
				var raw PackageJSONRaw
				if utils.ParseJSONFile(deonJsonPath, &raw) == nil {
					denoJson = raw.ToNpmPackage()
				}
			} else if deonJsoncPath := path.Join(installDir, "node_modules", pkg.Name, "deno.jsonc"); existsFile(deonJsoncPath) {
				data, err := os.ReadFile(deonJsoncPath)
				if err == nil {
					var raw PackageJSONRaw
					if json.Unmarshal(StripJSONC(data), &raw) == nil {
						denoJson = raw.ToNpmPackage()
					}
				}
			}
			if denoJson != nil {
				if len(denoJson.Imports) > 0 {
					buf.WriteString(`,"imports":{`)
					for k, v := range denoJson.Imports {
						if s, ok := v.(string); ok {
							buf.WriteString(`"` + k + `":"` + s + `",`)
						}
					}
					buf.Truncate(buf.Len() - 1)
					buf.WriteByte('}')
				}
				if denoJson.Exports.Len() > 0 {
					buf.WriteString(`,"exports":{`)
					for _, k := range denoJson.Exports.keys {
						if v, ok := denoJson.Exports.Get(k); ok {
							if s, ok := v.(string); ok {
								buf.WriteString(`"` + k + `":"` + s + `",`)
							}
						}
					}
					buf.Truncate(buf.Len() - 1)
					buf.WriteByte('}')
				}
			}
			buf.WriteByte('}')
			err = os.WriteFile(packageJsonPath, buf.Bytes(), 0644)
			if err != nil {
				return
			}
		}
	} else if pkg.PkgPrNew {
		err = fetchPackageTarball(&NpmRegistry{}, installDir, pkg.Name, "https://pkg.pr.new/"+pkg.Name+"@"+pkg.Version)
	} else {
		info, fetchErr := npmrc.getPackageInfo(pkg.Name, pkg.Version)
		if fetchErr != nil {
			return nil, fetchErr
		}
		if info.Deprecated != "" {
			os.WriteFile(path.Join(installDir, "deprecated.txt"), []byte(info.Deprecated), 0644)
		}
		err = fetchPackageTarball(npmrc.getRegistryByPackageName(pkg.Name), installDir, info.Name, info.Dist.Tarball)
	}
	if err != nil {
		return
	}

	err = utils.ParseJSONFile(packageJsonPath, &raw)
	if err != nil {
		os.RemoveAll(installDir)
		err = fmt.Errorf("failed to install %s: %v", pkg.String(), err)
		return
	}

	packageJson = raw.ToNpmPackage()
	return
}

func (npmrc *NpmRC) installDependencies(wd string, pkgJson *PackageJSON, npmMode bool, mark *set.Set[string]) {
	wg := sync.WaitGroup{}
	dependencies := map[string]string{}
	for name, version := range pkgJson.Dependencies {
		dependencies[name] = version
	}
	// install peer dependencies as well in _npm_ mode
	if npmMode {
		for name, version := range pkgJson.PeerDependencies {
			dependencies[name] = version
		}
	}
	if mark == nil {
		mark = set.New[string]()
	}
	for name, version := range dependencies {
		wg.Add(1)
		go func(name, version string) {
			defer wg.Done()
			pkg := Package{Name: name, Version: version}
			p, err := resolveDependencyVersion(version)
			if err != nil {
				return
			}
			if p.Name != "" {
				pkg = p
			}
			if strings.HasSuffix(pkg.Name, "@types/") {
				// skip installing `@types/*` packages
				return
			}
			if !isExactVersion(pkg.Version) && !pkg.Github && !pkg.PkgPrNew {
				p, e := npmrc.getPackageInfo(pkg.Name, pkg.Version)
				if e != nil {
					return
				}
				pkg.Version = p.Version
			}
			markId := fmt.Sprintf("%s@%s:%s:%v", pkgJson.Name, pkgJson.Version, pkg.String(), npmMode)
			if mark.Has(markId) {
				return
			}
			mark.Add(markId)
			installed, err := npmrc.installPackage(pkg)
			if err != nil {
				return
			}
			// link the installed package to the node_modules directory of current build context
			linkDir := path.Join(wd, "node_modules", name)
			_, err = os.Lstat(linkDir)
			if err != nil && os.IsNotExist(err) {
				if strings.ContainsRune(name, '/') {
					ensureDir(path.Dir(linkDir))
				}
				os.Symlink(path.Join(npmrc.StoreDir(), pkg.String(), "node_modules", pkg.Name), linkDir)
			}
			// install dependencies recursively
			if len(installed.Dependencies) > 0 || (len(installed.PeerDependencies) > 0 && npmMode) {
				npmrc.installDependencies(wd, installed, npmMode, mark)
			}
		}(name, version)
	}
	wg.Wait()
}

// If the package is deprecated, a depreacted.txt file will be created by the `intallPackage` function
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

func fetchPackageTarball(reg *NpmRegistry, installDir string, pkgName string, tarballUrl string) (err error) {
	u, err := url.Parse(tarballUrl)
	if err != nil {
		return
	}

	header := http.Header{}
	if reg.Token != "" {
		header.Set("Authorization", "Bearer "+reg.Token)
	} else if reg.User != "" && reg.Password != "" {
		header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(reg.User+":"+reg.Password)))
	}

	fetchClient, recycle := NewFetchClient(30, "esmd/"+VERSION, false)
	defer recycle()

	retryTimes := 0
RETRY:
	res, err := fetchClient.Fetch(u, header)
	if err != nil {
		if retryTimes < 3 {
			retryTimes++
			time.Sleep(time.Duration(retryTimes) * 100 * time.Millisecond)
			goto RETRY
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

	err = extractPackageTarball(installDir, pkgName, io.LimitReader(res.Body, maxPackageTarballSize))
	if err != nil {
		// clear installDir if failed to extract tarball
		os.RemoveAll(installDir)
	}
	return
}

func extractPackageTarball(installDir string, pkgName string, tarball io.Reader) (err error) {
	unziped, err := gzip.NewReader(tarball)
	if err != nil {
		return
	}

	pkgDir := path.Join(installDir, "node_modules", pkgName)

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
		filename := path.Join(pkgDir, name)
		if h.Typeflag != tar.TypeReg {
			continue
		}
		// ignore large files
		if h.Size > maxAssetFileSize {
			continue
		}
		extname := path.Ext(filename)
		if !(extname != "" && (assetExts[extname[1:]] || stringInSlice(moduleExts, extname) || extname == ".map" || extname == ".css" || extname == ".svelte" || extname == ".vue")) {
			// ignore unsupported formats
			continue
		}
		ensureDir(path.Dir(filename))
		f, err := os.OpenFile(filename, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer f.Close()
		n, err := io.Copy(f, tr)
		if err != nil {
			return err
		}
		if n != h.Size {
			return errors.New("extractPackageTarball: incomplete file: " + name)
		}
	}

	return nil
}

// resolveDependencyVersion resolves the version of a dependency
// e.g. "react": "npm:react@19.0.0"
// e.g. "react": "github:facebook/react#semver:19.0.0"
// e.g. "flag": "jsr:@luca/flag@0.0.1"
// e.g. "tinybench": "https://pkg.pr.new/tinybench@a832a55"
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
		if version == "" || !npmNaming.Match(version) {
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

func normalizePackageVersion(version string) string {
	// strip leading `=` or `v`
	if strings.HasPrefix(version, "=") {
		version = version[1:]
	} else if strings.HasPrefix(version, "v") && isExactVersion(version[1:]) {
		version = version[1:]
	}
	if version == "" || version == "*" {
		return "latest"
	}
	return version
}

func isDistTag(s string) bool {
	switch s {
	case "latest", "next", "beta", "alpha", "canary", "rc", "experimental":
		return true
	default:
		return false
	}
}

// isExactVersion returns true if the given version is an exact version.
func isExactVersion(version string) bool {
	a := strings.SplitN(version, ".", 3)
	if len(a) != 3 {
		return false
	}
	if len(a[0]) == 0 || !isNumericString(a[0]) || len(a[1]) == 0 || !isNumericString(a[1]) {
		return false
	}
	p := a[2]
	if len(p) == 0 {
		return false
	}
	patchEnd := false
	for i, c := range p {
		if !patchEnd {
			if c == '-' || c == '+' {
				if i == 0 || i == len(p)-1 {
					return false
				}
				patchEnd = true
			} else if c < '0' || c > '9' {
				return false
			}
		} else {
			if !(c == '.' || c == '_' || c == '-' || c == '+' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
				return false
			}
		}
	}
	return true
}

func isNumericString(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// based on https://github.com/npm/validate-npm-package-name
func validatePackageName(pkgName string) bool {
	if l := len(pkgName); l == 0 || l > 214 {
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

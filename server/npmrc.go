package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/esm-dev/esm.sh/internal/fetch"
	"github.com/esm-dev/esm.sh/internal/jsonc"
	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/goccy/go-json"
	"github.com/ije/gox/set"
	syncx "github.com/ije/gox/sync"
	"github.com/ije/gox/utils"
)

const (
	npmRegistry = "https://registry.npmjs.org/"
	jsrRegistry = "https://npm.jsr.io/"
)

var (
	defaultNpmRC *NpmRC
	installMutex syncx.KeyedMutex
)

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

func (npmrc *NpmRC) getPackageInfo(pkgName string, version string) (packageJson *npm.PackageJSON, err error) {
	reg := npmrc.getRegistryByPackageName(pkgName)
	getCacheKey := func(pkgName string, pkgVersion string) string {
		return reg.Registry + pkgName + "@" + pkgVersion
	}

	version = npm.NormalizePackageVersion(version)
	return withCache(getCacheKey(pkgName, version), time.Duration(config.NpmQueryCacheTTL)*time.Second, func() (*npm.PackageJSON, string, error) {
		// check if the package has been installed
		if !npm.IsDistTag(version) && npm.IsExactVersion(version) {
			var raw npm.PackageJSONRaw
			pkgJsonPath := path.Join(npmrc.StoreDir(), pkgName+"@"+version, "node_modules", pkgName, "package.json")
			if utils.ParseJSONFile(pkgJsonPath, &raw) == nil {
				return raw.ToNpmPackage(), "", nil
			}
		}

		regUrl := reg.Registry + pkgName
		isWellknownVersion := (npm.IsExactVersion(version) || npm.IsDistTag(version)) && strings.HasPrefix(regUrl, npmRegistry)
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

		fetchClient, recycle := fetch.NewClient("esmd/"+VERSION, 15, false)
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
			var raw npm.PackageJSONRaw
			err = json.NewDecoder(res.Body).Decode(&raw)
			if err != nil {
				return nil, "", err
			}
			return raw.ToNpmPackage(), getCacheKey(pkgName, raw.Version), nil
		}

		var metadata npm.PackageMetadata
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

func (npmrc *NpmRC) installPackage(pkg npm.Package) (packageJson *npm.PackageJSON, err error) {
	installDir := path.Join(npmrc.StoreDir(), pkg.String())
	packageJsonPath := path.Join(installDir, "node_modules", pkg.Name, "package.json")

	// check if the package has been installed
	var raw npm.PackageJSONRaw
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
			var denoJson *npm.PackageJSON
			if deonJsonPath := path.Join(installDir, "node_modules", pkg.Name, "deno.json"); existsFile(deonJsonPath) {
				var raw npm.PackageJSONRaw
				if utils.ParseJSONFile(deonJsonPath, &raw) == nil {
					denoJson = raw.ToNpmPackage()
				}
			} else if deonJsoncPath := path.Join(installDir, "node_modules", pkg.Name, "deno.jsonc"); existsFile(deonJsoncPath) {
				data, err := os.ReadFile(deonJsoncPath)
				if err == nil {
					var raw npm.PackageJSONRaw
					if json.Unmarshal(jsonc.StripJSONC(data), &raw) == nil {
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
					for _, k := range denoJson.Exports.Keys() {
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

func (npmrc *NpmRC) installDependencies(wd string, pkgJson *npm.PackageJSON, npmMode bool, mark *set.Set[string]) {
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
			pkg := npm.Package{Name: name, Version: version}
			p, err := npm.ResolveDependencyVersion(version)
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
			if !npm.IsExactVersion(pkg.Version) && !pkg.Github && !pkg.PkgPrNew {
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

	fetchClient, recycle := fetch.NewClient("esmd/"+VERSION, 30, false)
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
		if !(extname != "" && (assetExts[extname[1:]] || slices.Contains(moduleExts, extname) || extname == ".map" || extname == ".css" || extname == ".svelte" || extname == ".vue")) {
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

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
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/esm-dev/esm.sh/internal/fetch"
	"github.com/esm-dev/esm.sh/internal/jsonc"
	"github.com/esm-dev/esm.sh/internal/npm"
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
	NpmRegistryConfig
	versionRouteSupported atomic.Uint32
	rateLimited           atomic.Uint32
}

type NpmRC struct {
	globalRegistry   *NpmRegistry
	scopedRegistries map[string]*NpmRegistry
}

func DefaultNpmRC() *NpmRC {
	if defaultNpmRC != nil {
		return defaultNpmRC
	}
	globalRegistry := &NpmRegistry{
		NpmRegistryConfig: NpmRegistryConfig{
			Registry:       config.NpmRegistry,
			BackupRegistry: config.NpmBackupRegistry,
			Token:          config.NpmToken,
			User:           config.NpmUser,
			Password:       config.NpmPassword,
		},
	}
	defaultNpmRC = &NpmRC{
		globalRegistry: globalRegistry,
		scopedRegistries: map[string]*NpmRegistry{
			"@jsr": &NpmRegistry{
				NpmRegistryConfig: NpmRegistryConfig{
					Registry: jsrRegistry,
				},
			},
		},
	}
	if len(config.NpmScopedRegistries) > 0 {
		for scope, reg := range config.NpmScopedRegistries {
			defaultNpmRC.scopedRegistries[scope] = &NpmRegistry{
				NpmRegistryConfig: reg,
			}
		}
	}
	return defaultNpmRC
}

func (rc *NpmRC) StoreDir() string {
	return filepath.Join(config.WorkDir, "npm")
}

func (npmrc *NpmRC) getRegistryByPackageName(packageName string) *NpmRegistry {
	if strings.HasPrefix(packageName, "@") {
		scope, _ := utils.SplitByFirstByte(packageName, '/')
		reg, ok := npmrc.scopedRegistries[scope]
		if ok {
			return reg
		}
	}
	return npmrc.globalRegistry
}

func (npmrc *NpmRC) fetchPackageMetadata(pkgName string, version string, isWellknownVersion bool) (*npm.PackageMetadata, *npm.PackageJSONRaw, error) {
	reg := npmrc.getRegistryByPackageName(pkgName)
	regUrlStr := reg.Registry
	if reg.isRateLimited() && reg.BackupRegistry != "" {
		// use backup registry if the global registry is rate limited
		regUrlStr = reg.BackupRegistry
	}
	regUrlStr += pkgName

	var useVersionRoute bool
	if isWellknownVersion {
		useVersionRoute = reg.isSupportVersionRoute(regUrlStr)
		if useVersionRoute {
			regUrlStr += "/" + version
		}
	}

	regUrl, err := url.Parse(regUrlStr)
	if err != nil {
		return nil, nil, err
	}

	header := http.Header{}
	if reg.Token != "" {
		header.Set("Authorization", "Bearer "+reg.Token)
	} else if reg.User != "" && reg.Password != "" {
		header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(reg.User+":"+reg.Password)))
	}

	fetchClient, recycle := fetch.NewClient("esmd/"+VERSION, 15, false, nil)
	defer recycle()

	retryTimes := 0
RETRY:
	res, err := fetchClient.Fetch(regUrl, header)
	if err != nil {
		if retryTimes < 3 {
			retryTimes++
			time.Sleep(time.Duration(retryTimes) * 100 * time.Millisecond)
			goto RETRY
		}
		return nil, nil, err
	}
	defer res.Body.Close()

	if res.StatusCode == 404 || res.StatusCode == 401 {
		if isWellknownVersion {
			return nil, nil, fmt.Errorf("version %s of '%s' not found", version, pkgName)
		} else {
			return nil, nil, fmt.Errorf("package '%s' not found", pkgName)
		}
	}

	if res.StatusCode == 429 && reg.BackupRegistry != "" && !reg.isRateLimited() {
		reg.hitRateLimit()
		return npmrc.fetchPackageMetadata(pkgName, version, isWellknownVersion)
	}

	if res.StatusCode != 200 {
		return nil, nil, fmt.Errorf("%s: %s", regUrl.Hostname(), res.Status)
	}

	if isWellknownVersion && useVersionRoute {
		var raw npm.PackageJSONRaw
		err = json.NewDecoder(res.Body).Decode(&raw)
		if err != nil {
			return nil, nil, err
		}
		return nil, &raw, nil
	}

	var metadata npm.PackageMetadata
	err = json.NewDecoder(res.Body).Decode(&metadata)
	if err != nil {
		return nil, nil, err
	}

	if len(metadata.Versions) == 0 {
		return nil, nil, fmt.Errorf("no versions found for package '%s'", pkgName)
	}

	return &metadata, nil, nil
}

func resolveSemverVersion(metadata *npm.PackageMetadata, version string) (string, error) {
CHECK:
	distVersion, ok := metadata.DistTags[version]
	if ok {
		_, ok := metadata.Versions[distVersion]
		if ok {
			return distVersion, nil
		}
	} else {
		if version == "lastest" {
			return "", fmt.Errorf("version %s not found", version)
		}
		c, err := semver.NewConstraint(version)
		if err != nil {
			version = "latest"
			goto CHECK
		}
		vs := make([]*semver.Version, len(metadata.Versions))
		i := 0
		for v := range metadata.Versions {
			if !strings.ContainsRune(version, '-') && strings.ContainsRune(v, '-') {
				continue
			}
			sv, err := semver.NewVersion(v)
			if err == nil && c.Check(sv) {
				vs[i] = sv
				i++
			}
		}
		if i > 0 {
			vs = vs[:i]
			if i > 1 {
				sort.Sort(semver.Collection(vs))
			}
			return vs[i-1].String(), nil
		}
	}
	return "", fmt.Errorf("version %s not found", version)
}

func (npmrc *NpmRC) getPackageInfo(pkgName string, version string) (packageJson *npm.PackageJSON, err error) {
	reg := npmrc.getRegistryByPackageName(pkgName)
	getCacheKey := func(pkgName string, pkgVersion string) string {
		return reg.Registry + pkgName + "@" + pkgVersion
	}

	version = npm.NormalizePackageVersion(version)
	return withCache(getCacheKey(pkgName, version), time.Duration(config.NpmQueryCacheTTL)*time.Second, func() (*npm.PackageJSON, string, error) {
		if !npm.IsDistTag(version) && npm.IsExactVersion(version) {
			var raw npm.PackageJSONRaw
			pkgJsonPath := filepath.Join(npmrc.StoreDir(), pkgName+"@"+version, "node_modules", pkgName, "package.json")
			if utils.ParseJSONFile(pkgJsonPath, &raw) == nil {
				return raw.ToNpmPackage(), "", nil
			}
		}

		isWellknownVersion := npm.IsExactVersion(version) || npm.IsDistTag(version)
		metadata, raw, err := npmrc.fetchPackageMetadata(pkgName, version, isWellknownVersion)
		if err != nil {
			return nil, "", err
		}

		if raw != nil {
			return raw.ToNpmPackage(), getCacheKey(pkgName, raw.Version), nil
		}

		resolvedVersion, err := resolveSemverVersion(metadata, version)
		if err != nil {
			return nil, "", fmt.Errorf("version %s of '%s' not found", version, pkgName)
		}

		rawData, ok := metadata.Versions[resolvedVersion]
		if !ok {
			return nil, "", fmt.Errorf("version %s of '%s' not found", version, pkgName)
		}

		return rawData.ToNpmPackage(), getCacheKey(pkgName, rawData.Version), nil
	})
}

func (npmrc *NpmRC) getPackageInfoByDate(pkgName string, dateVersion string) (packageJson *npm.PackageJSON, err error) {
	targetTime, err := npm.ConvertDateVersionToTime(dateVersion)
	if err != nil {
		return nil, err
	}

	reg := npmrc.getRegistryByPackageName(pkgName)
	cacheKey := reg.Registry + pkgName + "@date=" + dateVersion

	return withCache(cacheKey, time.Duration(config.NpmQueryCacheTTL)*time.Second, func() (*npm.PackageJSON, string, error) {
		metadata, _, err := npmrc.fetchPackageMetadata(pkgName, dateVersion, false)
		if err != nil {
			return nil, "", err
		}

		resolvedVersion, err := npm.ResolveVersionByTime(metadata, targetTime)
		if err != nil {
			return nil, "", fmt.Errorf("date-based version resolution failed for %s@%s: %s", pkgName, dateVersion, err.Error())
		}

		raw, ok := metadata.Versions[resolvedVersion]
		if !ok {
			return nil, "", fmt.Errorf("resolved version %s of '%s' not found", resolvedVersion, pkgName)
		}

		exactVersionCacheKey := reg.Registry + pkgName + "@" + raw.Version
		return raw.ToNpmPackage(), exactVersionCacheKey, nil
	})
}

func (npmrc *NpmRC) installPackage(pkg npm.Package) (packageJson *npm.PackageJSON, err error) {
	installDir := filepath.Join(npmrc.StoreDir(), pkg.String())
	packageJsonPath := filepath.Join(installDir, "node_modules", pkg.Name, "package.json")

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
			if deonJsonPath := filepath.Join(installDir, "node_modules", pkg.Name, "deno.json"); existsFile(deonJsonPath) {
				var raw npm.PackageJSONRaw
				if utils.ParseJSONFile(deonJsonPath, &raw) == nil {
					denoJson = raw.ToNpmPackage()
				}
			} else if deonJsoncPath := filepath.Join(installDir, "node_modules", pkg.Name, "deno.jsonc"); existsFile(deonJsoncPath) {
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
			os.WriteFile(filepath.Join(installDir, "deprecated.txt"), []byte(info.Deprecated), 0644)
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
	maps.Copy(dependencies, pkgJson.Dependencies)
	// install peer dependencies if `npmMode` is true
	if npmMode {
		maps.Copy(dependencies, pkgJson.PeerDependencies)
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
			if err != nil || p.Url != "" {
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
			linkDir := filepath.Join(wd, "node_modules", name)
			_, err = os.Lstat(linkDir)
			if err != nil && os.IsNotExist(err) {
				if strings.ContainsRune(name, '/') {
					ensureDir(filepath.Dir(linkDir))
				}
				os.Symlink(filepath.Join(npmrc.StoreDir(), pkg.String(), "node_modules", pkg.Name), linkDir)
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
	installDir := filepath.Join(npmrc.StoreDir(), pkgName+"@"+pkgVersion)
	data, err := os.ReadFile(filepath.Join(installDir, "deprecated.txt"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

func (reg *NpmRegistry) isRateLimited() bool {
	return reg.rateLimited.Load() == 1
}

func (reg *NpmRegistry) hitRateLimit() {
	reg.rateLimited.Store(1)
	time.AfterFunc(30*time.Second, func() {
		reg.rateLimited.Store(0)
	})
}

// check if the registry supports the version route
// example: https://registry.npmjs.org/react/19.0.0
func (reg *NpmRegistry) isSupportVersionRoute(urlStr string) bool {
	if strings.HasPrefix(urlStr, npmRegistry) {
		return true
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	if reg.versionRouteSupported.Load() == 1 {
		return true
	}

	fetchClient, recycle := fetch.NewClient("esmd/"+VERSION, 15, false, nil)
	defer recycle()

	u.Path = "/react/19.0.0"
	res, err := fetchClient.Fetch(u, nil)
	if err != nil {
		return false
	}

	defer res.Body.Close()
	if res.StatusCode == 200 {
		reg.versionRouteSupported.Store(1)
		return true
	}
	return false
}

func fetchPackageTarball(reg *NpmRegistry, installDir string, pkgName string, tarballUrlStr string) (err error) {
	tarballUrl, err := url.Parse(tarballUrlStr)
	if err != nil {
		return
	}

	if reg.isRateLimited() && reg.BackupRegistry != "" && strings.HasPrefix(tarballUrlStr, reg.Registry) {
		var backupUrl *url.URL
		backupUrl, err = url.Parse(reg.BackupRegistry)
		if err != nil {
			return
		}
		backupUrl.Path = tarballUrl.Path
		backupUrl.RawQuery = tarballUrl.RawQuery
		tarballUrl = backupUrl
		tarballUrlStr = backupUrl.String()
	}

	header := http.Header{}
	if reg.Token != "" {
		header.Set("Authorization", "Bearer "+reg.Token)
	} else if reg.User != "" && reg.Password != "" {
		header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(reg.User+":"+reg.Password)))
	}

	fetchClient, recycle := fetch.NewClient("esmd/"+VERSION, 30, false, nil)
	defer recycle()

	retryTimes := 0
RETRY:
	res, err := fetchClient.Fetch(tarballUrl, header)
	if err != nil {
		if retryTimes < 3 {
			retryTimes++
			time.Sleep(time.Duration(retryTimes) * 100 * time.Millisecond)
			goto RETRY
		}
		err = fmt.Errorf("failed to download tarball of package '%s': %v", pkgName, err)
		return
	}
	defer res.Body.Close()

	if res.StatusCode == 404 || res.StatusCode == 401 {
		err = fmt.Errorf("tarball of package '%s' not found", pkgName)
		return
	}

	if res.StatusCode == 429 && reg.isRateLimited() && reg.BackupRegistry != "" && strings.HasPrefix(tarballUrlStr, reg.Registry) {
		var backupUrl *url.URL
		backupUrl, err = url.Parse(reg.BackupRegistry)
		if err != nil {
			return
		}
		backupUrl.Path = tarballUrl.Path
		backupUrl.RawQuery = tarballUrl.RawQuery
		tarballUrl = backupUrl
		tarballUrlStr = backupUrl.String()
		reg.hitRateLimit()
		goto RETRY
	}

	if res.StatusCode != 200 {
		err = fmt.Errorf("could not download tarball of package '%s': %s", pkgName, res.Status)
		return
	}

	err = extractPackageTarball(installDir, pkgName, io.LimitReader(res.Body, maxPackageTarballSize))
	if err != nil {
		err = fmt.Errorf("failed to extract tarball of package '%s': %v", pkgName, err)
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

	pkgDir := filepath.Join(installDir, "node_modules", pkgName)

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
		if h.Typeflag != tar.TypeReg {
			continue
		}
		// ignore large files
		if h.Size > maxAssetFileSize {
			continue
		}
		// normalize the filename
		_, filename := utils.SplitByFirstByte(utils.NormalizePathname(h.Name)[1:], '/')
		if filename == "" {
			continue
		}
		savepath := filepath.Join(pkgDir, filename)
		extname := filepath.Ext(savepath)
		if !(extname != "" && (assetExts[extname[1:]] || slices.Contains(moduleExts, extname) || extname == ".map" || extname == ".css" || extname == ".svelte" || extname == ".vue")) {
			// ignore unsupported formats
			continue
		}
		ensureDir(filepath.Dir(savepath))
		f, err := os.OpenFile(savepath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer f.Close()
		n, err := io.Copy(f, tr)
		if err != nil {
			return err
		}
		if n != h.Size {
			return errors.New("extractPackageTarball: incomplete file: " + savepath)
		}
	}

	return nil
}

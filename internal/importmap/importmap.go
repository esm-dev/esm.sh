package importmap

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/goccy/go-json"
	"github.com/ije/gox/sync"
	"github.com/ije/gox/term"
	"github.com/ije/gox/utils"
)

type ImportMap struct {
	Src       string                       `json:"$src,omitempty"`
	Cdn       string                       `json:"$cdn,omitempty"`
	Imports   map[string]string            `json:"imports,omitempty"`
	Scopes    map[string]map[string]string `json:"scopes,omitempty"`
	Routes    map[string]string            `json:"routes,omitempty"`
	Integrity map[string]string            `json:"integrity,omitempty"`
	srcUrl    *url.URL
}

func (im *ImportMap) Resolve(path string) (string, bool) {
	var query string
	path, query = utils.SplitByFirstByte(path, '?')
	if query != "" {
		query = "?" + query
	}
	imports := im.Imports
	if im.srcUrl == nil && im.Src != "" {
		im.srcUrl, _ = url.Parse(im.Src)
	}
	// todo: check `scopes`
	if len(imports) > 0 {
		if v, ok := imports[path]; ok {
			return im.toAbsPath(v) + query, true
		}
		if strings.ContainsRune(path, '/') {
			nonTrailingSlashImports := make([][2]string, 0, len(imports))
			for k, v := range imports {
				if strings.HasSuffix(k, "/") {
					if strings.HasPrefix(path, k) {
						return im.toAbsPath(v+path[len(k):]) + query, true
					}
				} else {
					nonTrailingSlashImports = append(nonTrailingSlashImports, [2]string{k, v})
				}
			}
			// expand match
			// e.g. `"react": "https://esm.sh/react@18` -> `"react/": "https://esm.sh/react@18/`
			for _, p := range nonTrailingSlashImports {
				k, v := p[0], p[1]
				p, q := utils.SplitByLastByte(v, '?')
				if q != "" {
					q = "?" + q
					if query != "" {
						q += "&" + query[1:]
					}
				} else if query != "" {
					q = query
				}
				if strings.HasPrefix(path, k+"/") {
					return im.toAbsPath(p+path[len(k):]) + q, true
				}
			}
		}
	}
	return path + query, false
}

func (im *ImportMap) toAbsPath(path string) string {
	if strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../") {
		if im.srcUrl != nil {
			return im.srcUrl.ResolveReference(&url.URL{Path: path}).String()
		}
		return path
	}
	return path
}

func (im *ImportMap) AddPackages(packages []string) bool {
	cdnOrigin := im.Cdn
	if cdnOrigin == "" {
		cdnOrigin = "https://esm.sh"
	}

	var resolvedPackages []PackageJSON
	var errors []error
	var wg sync.WaitGroup
	for _, pkg := range packages {
		wg.Add(1)
		go func(pkg string) {
			defer wg.Done()
			scopeName := ""
			pkgName := pkg
			if pkg[0] == '@' {
				scopeName, pkgName = utils.SplitByFirstByte(pkg[1:], '/')
			}
			pkgName, version := utils.SplitByFirstByte(pkgName, '@')
			if !npm.Naming.Match(pkgName) || !(scopeName == "" || npm.Naming.Match(scopeName[1:])) || !(version == "" || npm.Versioning.Match(version)) {
				errors = append(errors, fmt.Errorf("invalid package name or version: %s", pkg))
				return
			}
			pkgJson, err := fetchPackageInfo(cdnOrigin, pkg)
			if err != nil {
				errors = append(errors, err)
				return
			}
			resolvedPackages = append(resolvedPackages, pkgJson)
		}(pkg)
	}
	wg.Wait()

	if len(errors) > 0 {
		for _, err := range errors {
			fmt.Println(term.Red("✖︎"), err.Error())
		}
		return false
	}

	if im.Imports == nil {
		im.Imports = map[string]string{}
	}
	if im.Scopes == nil {
		im.Scopes = map[string]map[string]string{}
	}

	cdnScopeImports, hasCdnScopedImports := im.Scopes[cdnOrigin+"/"]
	if !hasCdnScopedImports {
		cdnScopeImports = map[string]string{}
		im.Scopes[cdnOrigin+"/"] = cdnScopeImports
	}
	for _, pkg := range resolvedPackages {
		url := cdnOrigin + "/"
		if len(pkg.Dependencies) > 0 || len(pkg.PeerDependencies) > 0 {
			url += "*" // externall deps marker
		}
		url += pkg.Name + "@" + pkg.Version
		im.Imports[pkg.Name] = url
		im.Imports[pkg.Name+"/"] = url + "/"
		if hasCdnScopedImports {
			delete(cdnScopeImports, pkg.Name)
			delete(cdnScopeImports, pkg.Name+"/")
		}
	}
	for _, pkg := range resolvedPackages {
		walkPackageDependencies(pkg, func(specifier, pkgName, pkgVersion, prefix string) {
			if _, ok := im.Imports[specifier]; !ok {
				prevUrl, prev := cdnScopeImports[specifier]
				deepCheck := true
			checkPrevUrl:
				if prev {
					pathname := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(prevUrl, cdnOrigin+"/"), "*"), "@")
					_, prevVersion := utils.SplitByFirstByte(pathname, '@')
					prevVersion, _ = utils.SplitByFirstByte(prevVersion, '/')
					if npm.IsExactVersion(prevVersion) {
						if npm.IsExactVersion(pkgVersion) {
							if pkgVersion == prevVersion {
								if _, ok := cdnScopeImports[specifier+"/"]; !ok {
									cdnScopeImports[specifier+"/"] = prevUrl + "/"
								}
								return
							}
						} else {
							c, err := semver.NewConstraint(pkgVersion)
							if err == nil && c.Check(semver.MustParse(prevVersion)) {
								if _, ok := cdnScopeImports[specifier+"/"]; !ok {
									cdnScopeImports[specifier+"/"] = prevUrl + "/"
								}
								return
							}
						}
					}
				}
				if deepCheck {
					if scopeImports, ok := im.Scopes[cdnOrigin+"/"+pkg.Name+"@"+pkg.Version+"/"]; ok {
						prevUrl, prev = scopeImports[specifier]
						if prev {
							deepCheck = false
							goto checkPrevUrl
						}
					}
				}
				p, err := fetchPackageInfo(cdnOrigin, pkgName+"@"+pkgVersion)
				if err != nil {
					errors = append(errors, err)
					return
				}
				url := cdnOrigin + prefix + "/"
				if len(p.Dependencies) > 0 || len(p.PeerDependencies) > 0 {
					url += "*"
				}
				url += pkgName + "@" + p.Version
				cdnScopeImports[specifier] = url
				cdnScopeImports[specifier+"/"] = url + "/"
			}
		})
	}
	if len(errors) > 0 {
		for _, err := range errors {
			fmt.Println(term.Red("✖︎"), err.Error())
		}
		return false
	}
	for _, pkg := range resolvedPackages {
		fmt.Println(term.Green("✔"), pkg.Name+term.Dim("@"+pkg.Version))
	}
	return true
}

func (im *ImportMap) MarshalJSON() ([]byte, error) {
	buf := bytes.Buffer{}
	buf.WriteString("{\n")
	if im.Cdn != "" {
		buf.WriteString("      \"$cdn\": \"")
		buf.WriteString(im.Cdn)
		buf.WriteString("\",\n")
	}
	buf.WriteString("      \"imports\": {")
	if len(im.Imports) > 0 {
		buf.WriteByte('\n')
		formatImports(&buf, im.Imports, 4)
		buf.WriteString("      }")
	} else {
		buf.WriteString("}")
	}
	if len(im.Scopes) > 0 {
		buf.WriteString(",\n      \"scopes\": {\n")
		for scope, imports := range im.Scopes {
			buf.WriteString("        \"")
			buf.WriteString(scope)
			buf.WriteString("\": {\n")
			formatImports(&buf, imports, 5)
			buf.WriteString("        }\n")
		}
		buf.WriteString("      }")
	}
	if len(im.Routes) > 0 {
		buf.WriteString(",\n      \"routes\": {\n")
		formatImports(&buf, im.Routes, 4)
		buf.WriteString("      }")
	}
	if len(im.Integrity) > 0 {
		buf.WriteString(",\n      \"integrity\": {\n")
		formatImports(&buf, im.Integrity, 4)
		buf.WriteString("      }")
	}
	buf.WriteString("\n    }")
	return buf.Bytes(), nil
}

func formatImports[T any](buf *bytes.Buffer, m map[string]T, indent int) {
	keys := make([]string, 0, len(m))
	for key := range m {
		if keyLen := len(key); keyLen == 1 || (keyLen > 1 && !strings.HasSuffix(key, "/")) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for i, key := range keys {
		value, ok := any(m[key]).(string)
		if !ok || value == "" {
			// ignore non-string value
			continue
		}
		buf.WriteString(strings.Repeat("  ", indent))
		buf.WriteByte('"')
		buf.WriteString(key)
		buf.WriteString("\": \"")
		buf.WriteString(value)
		buf.WriteByte('"')
		if value, ok := m[key+"/"]; ok {
			if str, ok := any(value).(string); ok && str != "" {
				buf.WriteString(",\n")
				buf.WriteString(strings.Repeat("  ", indent))
				buf.WriteByte('"')
				buf.WriteString(key + "/")
				buf.WriteString("\": \"")
				buf.WriteString(str)
				buf.WriteByte('"')
			}
		}
		if i < len(keys)-1 {
			buf.WriteString(",")
		}
		buf.WriteByte('\n')
	}
}

type PackageJSON struct {
	Name             string            `json:"name"`
	Version          string            `json:"version"`
	Dependencies     map[string]string `json:"dependencies"`
	PeerDependencies map[string]string `json:"peerDependencies"`
}

var (
	cacheMutex sync.KeyedMutex
	cacheStore sync.Map
)

func fetchPackageInfo(cdnOrigin string, pkg string) (pkgJSON PackageJSON, err error) {
	url := fmt.Sprintf("%s/%s/package.json", cdnOrigin, pkg)

	// check cache first
	if v, ok := cacheStore.Load(url); ok {
		pkgJSON, _ = v.(PackageJSON)
		return
	}

	// only one fetch at a time for the same url
	unlock := cacheMutex.Lock(url)
	defer unlock()

	// check cache again after get lock
	if v, ok := cacheStore.Load(url); ok {
		pkgJSON, _ = v.(PackageJSON)
		return
	}

	resp, err := http.Get(url)
	if err != nil {
		err = errors.New("http request failed: " + err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		err = errors.New("package not found: " + pkg)
		return
	}
	if resp.StatusCode != 200 {
		msg, _ := io.ReadAll(resp.Body)
		err = errors.New(string(msg))
		return
	}

	err = json.NewDecoder(resp.Body).Decode(&pkgJSON)
	if err != nil {
		err = errors.New("could not parse package.json")
	}
	if err == nil {
		cacheStore.Store(url, pkgJSON)
	}
	return
}

func walkPackageDependencies(pkg PackageJSON, callback func(specifier, pkgName, pkgVersion, prefix string)) {
	if len(pkg.Dependencies) > 0 {
		walkDependencies(pkg.Dependencies, callback)
	}
	if len(pkg.PeerDependencies) > 0 {
		walkDependencies(pkg.PeerDependencies, callback)
	}
}

func walkDependencies(deps map[string]string, callback func(specifier, pkgName, pkgVersion, prefix string)) {
	for specifier, pkgVersion := range deps {
		pkgName := specifier
		pkg, err := npm.ResolveDependencyVersion(pkgVersion)
		if err == nil && pkg.Name != "" {
			pkgName = pkg.Name
			pkgVersion = pkg.Version
		}
		var prefix string
		if pkg.Github {
			prefix = "/gh"
		} else if pkg.PkgPrNew {
			prefix = "/pr"
		} else if pkg.Tgz {
			prefix = "/tgz"
		}
		callback(specifier, pkgName, pkgVersion, prefix)
	}
}

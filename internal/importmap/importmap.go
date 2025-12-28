package importmap

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/Masterminds/semver/v3"
	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/ije/gox/set"
	"github.com/ije/gox/term"
	"github.com/ije/gox/utils"
	"golang.org/x/net/html"
)

type ImportMap struct {
	Src       string                       `json:"$src,omitempty"`
	Cdn       string                       `json:"$cdn,omitempty"`
	Imports   map[string]string            `json:"imports,omitempty"`
	Scopes    map[string]map[string]string `json:"scopes,omitempty"`
	Routes    map[string]string            `json:"routes,omitempty"`
	Integrity map[string]string            `json:"integrity,omitempty"`
	baseUrl   *url.URL                     // cached base URL
}

// ParseFromHtmlFile parses an import map from an HTML file.
func ParseFromHtmlFile(filename string) (importMap ImportMap, err error) {
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()

	tokenizer := html.NewTokenizer(file)
	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt == html.StartTagToken {
			tagName, moreAttr := tokenizer.TagName()
			if string(tagName) == "script" {
				var typeAttr string
				for moreAttr {
					var key, val []byte
					key, val, moreAttr = tokenizer.TagAttr()
					if string(key) == "type" {
						typeAttr = string(val)
						break
					}
				}
				if typeAttr == "importmap" {
					if tokenizer.Next() != html.TextToken {
						err = errors.New("invalid import map")
						return
					}
					if json.Unmarshal(tokenizer.Raw(), &importMap) != nil {
						err = errors.New("invalid import map")
						return
					}
					importMap.Src = "file://" + string(filename)
					break
				}
			} else if string(tagName) == "body" {
				// stop parsing when we reach the body tag
				break
			}
		} else if tt == html.EndTagToken {
			tagName, _ := tokenizer.TagName()
			if bytes.Equal(tagName, []byte("head")) {
				// stop parsing when we reach the head end tag
				break
			}
		}
	}
	return
}

func (im *ImportMap) Resolve(path string) (string, bool) {
	var query string
	path, query = utils.SplitByFirstByte(path, '?')
	if query != "" {
		query = "?" + query
	}
	imports := im.Imports
	if im.baseUrl == nil && im.Src != "" {
		im.baseUrl, _ = url.Parse(im.Src)
	}
	// todo: check `scopes`
	if len(imports) > 0 {
		if v, ok := imports[path]; ok {
			return normalizeUrl(im.baseUrl, v) + query, true
		}
		if strings.ContainsRune(path, '/') {
			nonTrailingSlashImports := make([][2]string, 0, len(imports))
			for k, v := range imports {
				if strings.HasSuffix(k, "/") {
					if strings.HasPrefix(path, k) {
						return normalizeUrl(im.baseUrl, v+path[len(k):]) + query, true
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
					return normalizeUrl(im.baseUrl, p+path[len(k):]) + q, true
				}
			}
		}
	}
	return path + query, false
}

func (im *ImportMap) AddPackages(packages []string, expandMode bool) (updated bool) {
	cdnOrigin := im.cdnOrign()
	resolvedPackages := make([]PackageInfo, 0, len(packages))

	var wg sync.WaitGroup
	var errs []error

	for _, specifier := range packages {
		wg.Add(1)
		go func(specifier string) {
			defer wg.Done()
			var scopeName string
			var pkgName string
			var regPrefix string
			if strings.HasPrefix(specifier, "jsr:") {
				regPrefix = "jsr/"
				specifier = specifier[4:]
			} else if strings.ContainsRune(specifier, '/') && specifier[0] != '@' {
				regPrefix = "gh/"
				specifier = strings.Replace(specifier, "#", "@", 1) // owner/repo#branch -> owner/repo@branch
			}
			if len(specifier) > 0 && (specifier[0] == '@' || regPrefix == "gh/") {
				scopeName, pkgName = utils.SplitByFirstByte(specifier, '/')
			} else {
				pkgName = specifier
			}
			if pkgName == "" {
				// ignore empty package name
				return
			}
			pkgName, version := utils.SplitByFirstByte(pkgName, '@')
			if pkgName == "" || !npm.Naming.Match(pkgName) || !(scopeName == "" || npm.Naming.Match(strings.TrimPrefix(scopeName, "@"))) || !(version == "" || npm.Versioning.Match(version)) {
				errs = append(errs, fmt.Errorf("invalid package name or version: %s", specifier))
				return
			}
			if scopeName != "" {
				pkgName = scopeName + "/" + pkgName
			}
			pkgJson, err := fetchPackageInfo(cdnOrigin, regPrefix, pkgName, version)
			if err != nil {
				errs = append(errs, err)
				return
			}
			resolvedPackages = append(resolvedPackages, pkgJson)
		}(specifier)
	}

	wg.Wait()

	if len(errs) > 0 {
		for _, err := range errs {
			fmt.Println(term.Red("✖︎"), err.Error())
		}
		return false
	}

	marker := set.New[string]()
	for _, pkg := range resolvedPackages {
		errs := im.addPackage(pkg.Name, pkg, expandMode, nil, marker)
		if len(errs) > 0 {
			for _, err := range errs {
				fmt.Println(term.Red("✖︎"), err.Error())
			}
			return false
		}
	}

	installed := make([]string, 0, len(resolvedPackages))
	for _, pkg := range resolvedPackages {
		installed = append(installed, pkg.Name+term.Dim("@"+pkg.Version))
	}
	sort.Strings(installed)
	for _, pkg := range installed {
		fmt.Println(term.Green("✔"), pkg)
	}
	return true
}

func (im *ImportMap) addPackage(specifier string, pkg PackageInfo, expandMode bool, targetImports map[string]string, marker *set.Set[string]) (errs []error) {
	markId := pkg.String()
	if marker.Has(markId) {
		return
	}
	marker.Add(markId)

	if im.Imports == nil {
		im.Imports = map[string]string{}
	}
	if im.Scopes == nil {
		im.Scopes = map[string]map[string]string{}
	}

	cdnOrigin := im.cdnOrign()
	cdnScopeImports, cdnScoped := im.Scopes[cdnOrigin+"/"]
	if !cdnScoped {
		cdnScopeImports = map[string]string{}
		im.Scopes[cdnOrigin+"/"] = cdnScopeImports
	}

	indirect := true
	if targetImports == nil {
		indirect = false
		targetImports = im.Imports
	}

	url := cdnOrigin + "/"
	if pkg.Github {
		url += "gh/"
	} else if pkg.Jsr {
		url += "jsr/"
	}
	if len(pkg.Dependencies) > 0 || len(pkg.PeerDependencies) > 0 {
		url += "*" // external all modifier
	}
	url += pkg.Name + "@" + pkg.Version
	targetImports[specifier] = url
	if !expandMode {
		targetImports[specifier+"/"] = url + "/"
	}
	if !indirect {
		delete(cdnScopeImports, specifier)
		delete(cdnScopeImports, specifier+"/")
	}

	var deps []Dependency
	walkPackageDependencies(pkg, func(dep Dependency) {
		// if the version of the dependency is not exact,
		// check if it is satisfied with the version in the import map
		if !npm.IsExactVersion(dep.Version) {
			importUrl, exists := im.Imports[pkg.Name]
			if !exists || strings.HasPrefix(importUrl, cdnOrigin+"/") {
				importUrl, exists = cdnScopeImports[pkg.Name]
			}
			if exists && strings.HasPrefix(importUrl, cdnOrigin+"/") {
				var version string
				if npm.IsExactVersion(version) {
					c, err := semver.NewConstraint(dep.Version)
					if err == nil && c.Check(semver.MustParse(version)) {
						dep.Version = version
					}
				}
			}
		}
		deps = append(deps, dep)
	})

	return
}

func (im *ImportMap) cdnOrign() string {
	cdnOrigin := im.Cdn
	if cdnOrigin == "" {
		return "https://esm.sh"
	}
	return cdnOrigin
}

func (im *ImportMap) MarshalJSON() ([]byte, error) {
	return []byte(im.FormatJSON(0)), nil
}

func (im *ImportMap) FormatJSON(indent int) string {
	buf := strings.Builder{}
	indentStr := bytes.Repeat([]byte{' ', ' '}, indent+1)
	buf.Write(indentStr[0 : 2*indent])
	buf.WriteString("{\n")
	if im.Cdn != "" && im.Cdn != "https://esm.sh" {
		buf.Write(indentStr)
		buf.WriteString("\"$cdn\": \"")
		buf.WriteString(im.Cdn)
		buf.WriteString("\",\n")
	}
	buf.Write(indentStr)
	buf.WriteString("\"imports\": {")
	if len(im.Imports) > 0 {
		buf.WriteByte('\n')
		formatImports(&buf, im.Imports, indent+2)
		buf.Write(indentStr)
		buf.WriteByte('}')
	} else {
		buf.WriteByte('}')
	}
	hasScopeImports := false
	if len(im.Scopes) > 0 {
		for _, imports := range im.Scopes {
			if len(imports) > 0 {
				hasScopeImports = true
				break
			}
		}
	}
	if hasScopeImports {
		buf.WriteString(",\n")
		buf.Write(indentStr)
		buf.WriteString("\"scopes\": {\n")
		for scope, imports := range im.Scopes {
			buf.Write(indentStr)
			buf.WriteString("  \"")
			buf.WriteString(scope)
			buf.WriteString("\": {\n")
			formatImports(&buf, imports, indent+3)
			buf.Write(indentStr)
			buf.WriteString("  }\n")
		}
		buf.Write(indentStr)
		buf.WriteByte('}')
	}
	if len(im.Routes) > 0 {
		buf.WriteString(",\n")
		buf.Write(indentStr)
		buf.WriteString("\"routes\": {\n")
		formatMap(&buf, im.Routes, indent+2)
		buf.Write(indentStr)
		buf.WriteByte('}')
	}
	if len(im.Integrity) > 0 {
		buf.WriteString(",\n")
		buf.Write(indentStr)
		buf.WriteString("\"integrity\": {\n")
		formatMap(&buf, im.Integrity, indent+2)
		buf.Write(indentStr)
		buf.WriteByte('}')
	}
	buf.WriteByte('\n')
	buf.Write(indentStr[0 : 2*indent])
	buf.WriteByte('}')
	return buf.String()
}

func normalizeUrl(baseUrl *url.URL, path string) string {
	if baseUrl != nil && (strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../")) {
		return baseUrl.ResolveReference(&url.URL{Path: path}).String()
	}
	return path
}

func formatImports(buf *strings.Builder, imports map[string]string, indent int) {
	keys := make([]string, 0, len(imports))
	for key := range imports {
		if keyLen := len(key); keyLen == 1 || (keyLen > 1 && !strings.HasSuffix(key, "/")) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	indentStr := bytes.Repeat([]byte{' ', ' '}, indent)
	for i, key := range keys {
		url, ok := imports[key]
		if !ok || url == "" {
			// ignore empty values
			continue
		}
		buf.Write(indentStr)
		buf.WriteByte('"')
		buf.WriteString(key)
		buf.WriteString("\": \"")
		buf.WriteString(url)
		buf.WriteByte('"')
		if url, ok := imports[key+"/"]; ok && url != "" {
			buf.WriteString(",\n")
			buf.Write(indentStr)
			buf.WriteByte('"')
			buf.WriteString(key + "/")
			buf.WriteString("\": \"")
			buf.WriteString(url)
			buf.WriteByte('"')
		}
		if i < len(keys)-1 {
			buf.WriteByte(',')
		}
		buf.WriteByte('\n')
	}
}

func formatMap(buf *strings.Builder, m map[string]string, indent int) {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	indentStr := bytes.Repeat([]byte{' ', ' '}, indent)
	for i, key := range keys {
		value, ok := m[key]
		if !ok || value == "" {
			// ignore empty values
			continue
		}
		buf.Write(indentStr)
		buf.WriteByte('"')
		buf.WriteString(key)
		buf.WriteString("\": \"")
		buf.WriteString(value)
		buf.WriteByte('"')
		if i < len(keys)-1 {
			buf.WriteByte(',')
		}
		buf.WriteByte('\n')
	}
}

package importmap

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/goccy/go-json"
	"github.com/ije/gox/sync"
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

func (im *ImportMap) FormatJSON(indent int) string {
	buf := strings.Builder{}
	indentStr := bytes.Repeat([]byte{' ', ' '}, indent+1)
	buf.Write(indentStr[0 : 2*indent])
	buf.WriteString("{\n")
	if im.Cdn != "" {
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
	if len(im.Scopes) > 0 {
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

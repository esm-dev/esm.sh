package importmap

import (
	"bytes"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/Masterminds/semver/v3"
	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/ije/gox/term"
	"github.com/ije/gox/utils"
)

type Config struct {
	CDN    string `json:"cdn,omitempty"`
	Target string `json:"target,omitempty"`
	SRI    any    `json:"sri,omitempty"`
}

type SRIConfig struct {
	Algorithm string `json:"algorithm"`
}

type ImportMap struct {
	BaseUrl   string                       `json:"baseUrl,omitempty"`
	Imports   map[string]string            `json:"imports,omitempty"`
	Scopes    map[string]map[string]string `json:"scopes,omitempty"`
	Routes    map[string]string            `json:"routes,omitempty"`
	Integrity map[string]string            `json:"integrity,omitempty"`
	Config    Config                       `json:"config,omitempty"`
}

func (im *ImportMap) Resolve(specifier string, referrer *url.URL) (string, bool) {
	imports := im.Imports
	baseUrl, err := url.Parse(im.BaseUrl)
	if err != nil {
		return "", false
	}

	specifier, _ = utils.SplitByFirstByte(specifier, '#')
	var query string
	specifier, query = utils.SplitByFirstByte(specifier, '?')
	if query != "" {
		query = "?" + query
	}

	if referrer != nil {
		scopeKeys := make(ScopeKeys, 0, len(im.Scopes))
		for prefix := range im.Scopes {
			scopeKeys = append(scopeKeys, prefix)
		}
		sort.Sort(scopeKeys)
		for _, scopeKey := range scopeKeys {
			if strings.HasPrefix(referrer.String(), scopeKey) {
				imports = im.Scopes[scopeKey]
				break
			}
		}
	}

	if len(imports) > 0 {
		if v, ok := imports[specifier]; ok {
			return normalizeUrl(baseUrl, v) + query, true
		}
		if strings.ContainsRune(specifier, '/') {
			for k, v := range imports {
				if strings.HasSuffix(k, "/") {
					if strings.HasPrefix(specifier, k) {
						return normalizeUrl(baseUrl, v+specifier[len(k):]) + query, true
					}
				}
			}
		}
	}

	return specifier + query, false
}

func (im *ImportMap) AddPackages(packages []string) (addedPackages []PackageInfo, warnings []string, errors []error) {
	var wg sync.WaitGroup
	for _, specifier := range packages {
		wg.Go(func() {
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
				errors = append(errors, fmt.Errorf("invalid package name or version: %s", specifier))
				return
			}
			if scopeName != "" {
				pkgName = scopeName + "/" + pkgName
			}
			pkgJson, err := fetchPackageInfo(im.cdnOrign(), regPrefix, pkgName, version)
			if err != nil {
				errors = append(errors, err)
				return
			}
			addedPackages = append(addedPackages, pkgJson)
		})
	}
	wg.Wait()

	for _, pkg := range addedPackages {
		warns, errs := im.addPackage(pkg, false, nil)
		warnings = append(warnings, warns...)
		errors = append(errors, errs...)
	}
	return
}

func (im *ImportMap) addPackage(pkg PackageInfo, indirect bool, targetImportsMap map[string]string) (warnings []string, errors []error) {
	if im.Imports == nil {
		im.Imports = map[string]string{}
	}
	if im.Scopes == nil {
		im.Scopes = map[string]map[string]string{}
	}

	cdnOrigin := im.cdnOrign()
	cdnScopeImportsMap, cdnScoped := im.Scopes[cdnOrigin+"/"]
	if !cdnScoped {
		cdnScopeImportsMap = map[string]string{}
		im.Scopes[cdnOrigin+"/"] = cdnScopeImportsMap
	}

	importsMap := im.Imports
	if indirect {
		if targetImportsMap != nil {
			importsMap = targetImportsMap
		} else {
			importsMap = cdnScopeImportsMap
		}
	}

	var target string
	switch v := im.Config.Target; v {
	case "es2015", "es2016", "es2017", "es2018", "es2019", "es2020", "es2021", "es2022", "es2023", "es2024", "esnext":
		target = v
	default:
		target = "es2022"
	}

	baseUrl := cdnOrigin + "/" + pkg.String()
	if strings.HasSuffix(pkg.Name, "@") {
		_, nameNoScope := utils.SplitByFirstByte(pkg.Name, '/')
		importsMap[pkg.Name] = baseUrl + "/" + target + "/" + nameNoScope + ".mjs"
	} else {
		importsMap[pkg.Name] = baseUrl + "/" + target + "/" + pkg.Name + ".mjs"
	}
	importsMap[pkg.Name+"/"] = baseUrl + "&target=" + target + "/"
	if !indirect {
		delete(cdnScopeImportsMap, pkg.Name)
		delete(cdnScopeImportsMap, pkg.Name+"/")
	}

	deps, err := resolvePackageDependencies(pkg)
	if err != nil {
		fmt.Println(term.Red("[error]"), err.Error())
		return
	}

	wg := sync.WaitGroup{}
	for _, dep := range deps {
		wg.Go(func() {
			var targetImportsMap map[string]string
			// if the version of the dependency is not exact,
			// check if it is satisfied with the version in the import map
			// or create a new scope for the dependency
			importUrl, exists := im.Imports[dep.Name]
			if !exists {
				importUrl, exists = cdnScopeImportsMap[dep.Name]
			}
			if exists && strings.HasPrefix(importUrl, cdnOrigin+"/") {
				p, err := GetPackageInfoFromUrl(importUrl)
				if err == nil && npm.IsExactVersion(p.Version) {
					if dep.Version == p.Version {
						// the version of the dependency is exact and equals to the version in the import map
						return
					}
					if !npm.IsExactVersion(dep.Version) {
						c, err := semver.NewConstraint(dep.Version)
						if err == nil && c.Check(semver.MustParse(p.Version)) {
							// the version of the dependency is exact and satisfied with the version in the import map
							return
						}
						if dep.Peer {
							warnings = append(warnings, "incorrect peer dependency "+dep.Name+"@"+p.Version+term.Dim("(unmet "+dep.Version+")"))
							return
						}
						scope := cdnOrigin + "/" + pkg.String() + "/"
						ok := false
						targetImportsMap, ok = im.Scopes[scope]
						if !ok {
							targetImportsMap = map[string]string{}
							im.Scopes[scope] = targetImportsMap
						}
					}
				}
			}
			pkg, err := resolveDependency(cdnOrigin, dep)
			if err != nil {
				errors = append(errors, err)
				return
			}
			warns, errs := im.addPackage(pkg, !dep.Peer, targetImportsMap)
			warnings = append(warnings, warns...)
			errors = append(errors, errs...)
		})
	}
	wg.Wait()
	return
}

func (im *ImportMap) cdnOrign() string {
	cdn := im.Config.CDN
	if strings.HasPrefix(cdn, "https://") || strings.HasPrefix(cdn, "http://") {
		return cdn
	}
	return "https://esm.sh"
}

func (im *ImportMap) MarshalJSON() ([]byte, error) {
	return []byte(im.FormatJSON(0)), nil
}

func (im *ImportMap) FormatJSON(indent int) string {
	buf := strings.Builder{}
	indentStr := bytes.Repeat([]byte{' ', ' '}, indent+1)
	buf.Write(indentStr[0 : 2*indent])
	buf.WriteString("{\n")
	if im.Config.CDN != "" && im.Config.CDN != "https://esm.sh" {
		buf.Write(indentStr)
		buf.WriteString("\"config\": {\n")
		buf.Write(indentStr)
		buf.Write(indentStr)
		buf.WriteString("\"cdn\": \"")
		buf.WriteString(im.Config.CDN)
		buf.WriteString("\"\n")
		buf.Write(indentStr)
		buf.WriteString("}\n")
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
	scopes := make([]string, 0, len(im.Scopes))
	if len(im.Scopes) > 0 {
		for key, imports := range im.Scopes {
			if len(imports) > 0 {
				scopes = append(scopes, key)
			}
		}
	}
	sort.Strings(scopes)
	if len(scopes) > 0 {
		buf.WriteString(",\n")
		buf.Write(indentStr)
		buf.WriteString("\"scopes\": {\n")
		i := 0
		for _, scope := range scopes {
			imports := im.Scopes[scope]
			if len(imports) == 0 {
				continue
			}
			buf.Write(indentStr)
			buf.WriteString("  \"")
			buf.WriteString(scope)
			buf.WriteString("\": {\n")
			formatImports(&buf, imports, indent+3)
			buf.Write(indentStr)
			buf.WriteString("  }")
			if len(scopes) > 1 && i < len(scopes)-1 {
				buf.WriteByte(',')
			}
			buf.WriteByte('\n')
			i++
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

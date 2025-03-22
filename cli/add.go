package cli

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/esm-dev/esm.sh/server/common"
	"github.com/goccy/go-json"
	"github.com/ije/gox/sync"
	"github.com/ije/gox/term"
	"github.com/ije/gox/utils"
	"github.com/ije/gox/valid"
	"golang.org/x/net/html"
)

const addHelpMessage = "\033[30mesm.sh - A nobuild tool for modern web development.\033[0m" + `

Usage: esm.sh add [...packages] <options>

Examples:
  esm.sh add react@19.0.0
  esm.sh add react@19 react-dom@19
  esm.sh add react react-dom @esm.sh/router

Options:
  --help                 Show help message
`

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Hello, world!</title>
  <script type="importmap">
    %s
  </script>
</head>
<body>
  <h1>Hello, world!</h1>
</body>
</html>
`

const defaultCdnOrigin = "https://esm.sh"

// Add adds packages to "importmap" script
func Add() {
	help := flag.Bool("help", false, "Show help message")
	arg0, argMore := parseCommandFlag(2)

	if *help {
		fmt.Print(addHelpMessage)
		return
	}

	var packages []string
	if arg0 != "" {
		packages = append(packages, arg0)
		packages = append(packages, argMore...)
	}

	err := updateImportMap(packages)
	if err != nil {
		fmt.Println(term.Red("✖︎"), "Failed to add packages: "+err.Error())
	}
}

func updateImportMap(packages []string) (err error) {
	indexHtml, exists, err := lookupCloestFile("index.html")
	if err != nil {
		return
	}

	if exists {
		var f *os.File
		f, err = os.Open(indexHtml)
		if err != nil {
			return
		}
		tokenizer := html.NewTokenizer(f)
		buf := bytes.NewBuffer(nil)
		updated := false
		for {
			token := tokenizer.Next()
			if token == html.ErrorToken && tokenizer.Err() == io.EOF {
				break
			}
			if token == html.EndTagToken {
				tagName, _ := tokenizer.TagName()
				if string(tagName) == "head" && !updated {
					buf.WriteString("  <script type=\"importmap\">\n    ")
					importMap, ok := addPackagesToImportMap(defaultCdnOrigin, common.ImportMap{}, packages)
					if !ok {
						return
					}
					buf.Write([]byte(formatImportMap(defaultCdnOrigin, importMap)))
					buf.WriteString("\n  </script>\n")
					buf.Write(tokenizer.Raw())
					updated = true
					continue
				}
			}
			if token == html.StartTagToken {
				tagName, moreAttr := tokenizer.TagName()
				if string(tagName) == "script" && moreAttr {
					var typeAttr string
					for moreAttr {
						var key, val []byte
						key, val, moreAttr = tokenizer.TagAttr()
						if string(key) == "type" {
							typeAttr = string(val)
							break
						}
					}
					if typeAttr != "importmap" && !updated {
						buf.WriteString("<script type=\"importmap\">\n    ")
						importMap, ok := addPackagesToImportMap(defaultCdnOrigin, common.ImportMap{}, packages)
						if !ok {
							return
						}
						buf.Write([]byte(formatImportMap(defaultCdnOrigin, importMap)))
						buf.WriteString("\n  </script>\n  ")
						buf.Write(tokenizer.Raw())
						updated = true
						continue
					}
					if typeAttr == "importmap" && !updated {
						buf.Write(tokenizer.Raw())
						token := tokenizer.Next()
						cdnOrigin := defaultCdnOrigin
						var importMap common.ImportMap
						if token == html.TextToken {
							importMapRaw := bytes.TrimSpace(tokenizer.Text())
							if len(importMapRaw) > 0 {
								var o struct {
									CDN string `json:"cdn"`
								}
								if json.Unmarshal(importMapRaw, &o) == nil && (strings.HasPrefix(o.CDN, "https://") || strings.HasPrefix(o.CDN, "http://")) {
									cdnOrigin = o.CDN
								}
								if json.Unmarshal(importMapRaw, &importMap) != nil {
									err = fmt.Errorf("invalid importmap script")
									return
								}
							}
						}
						buf.WriteString("\n    ")
						importMap, ok := addPackagesToImportMap(cdnOrigin, importMap, packages)
						if !ok {
							return
						}
						buf.Write([]byte(formatImportMap(cdnOrigin, importMap)))
						buf.WriteString("\n  ")
						if token == html.EndTagToken {
							buf.Write(tokenizer.Raw())
						}
						updated = true
						continue
					}
				}
			}
			buf.Write(tokenizer.Raw())
		}
		fi, erro := f.Stat()
		f.Close()
		if erro != nil {
			return erro
		}
		err = os.WriteFile(indexHtml, buf.Bytes(), fi.Mode())
	} else {
		importMap, ok := addPackagesToImportMap(defaultCdnOrigin, common.ImportMap{}, packages)
		if !ok {
			return
		}
		err = os.WriteFile(indexHtml, fmt.Appendf(nil, htmlTemplate, formatImportMap(defaultCdnOrigin, importMap)), 0644)
		if err == nil {
			fmt.Println(term.Dim("Created index.html with importmap script."))
		}
	}
	return
}

func formatImportMap(cdnOrigin string, importMap common.ImportMap) string {
	buf := strings.Builder{}
	buf.WriteString("{\n      \"imports\": {")
	if len(importMap.Imports) > 0 {
		buf.WriteByte('\n')
		formatImports(&buf, importMap.Imports, 4)
		buf.WriteString("      }")
	} else {
		buf.WriteString("}")
	}
	if len(importMap.Scopes) > 0 {
		buf.WriteString(",\n      \"scopes\": {\n")
		for scope, imports := range importMap.Scopes {
			buf.WriteString("        \"")
			buf.WriteString(scope)
			buf.WriteString("\": {\n")
			formatImports(&buf, imports, 5)
			buf.WriteString("        }\n")
		}
		buf.WriteString("      }")
	}
	if len(importMap.Integrity) > 0 {
		buf.WriteString(",\n      \"integrity\": {\n")
		formatImports(&buf, importMap.Integrity, 4)
		buf.WriteString("      }")
	}
	if len(importMap.Routes) > 0 {
		buf.WriteString(",\n      \"routes\": {\n")
		formatImports(&buf, importMap.Routes, 4)
		buf.WriteString("      }")
	}
	buf.WriteString(",\n      \"cdn\": \"")
	buf.WriteString(cdnOrigin)
	buf.WriteString("\"\n    }")
	return buf.String()
}

func formatImports[T any](buf *strings.Builder, m map[string]T, indent int) {
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

func addPackagesToImportMap(cdnOrigin string, importMap common.ImportMap, packages []string) (common.ImportMap, bool) {
	npmNaming := valid.Validator{valid.Range{'a', 'z'}, valid.Range{'A', 'Z'}, valid.Range{'0', '9'}, valid.Eq('_'), valid.Eq('.'), valid.Eq('-'), valid.Eq('+'), valid.Eq('$'), valid.Eq('!')}
	npmVersioning := valid.Validator{valid.Range{'a', 'z'}, valid.Range{'A', 'Z'}, valid.Range{'0', '9'}, valid.Eq('_'), valid.Eq('.'), valid.Eq('-'), valid.Eq('+')}

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
			if !npmNaming.Match(pkgName) || !(scopeName == "" || npmNaming.Match(scopeName[1:])) || !(version == "" || npmVersioning.Match(version)) {
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
		return common.ImportMap{}, false
	}

	if importMap.Imports == nil {
		importMap.Imports = map[string]string{}
	}
	if importMap.Scopes == nil {
		importMap.Scopes = map[string]map[string]string{}
	}

	cdnScopeImports, hasCdnScopedImports := importMap.Scopes[cdnOrigin+"/"]
	if !hasCdnScopedImports {
		cdnScopeImports = map[string]string{}
		importMap.Scopes[cdnOrigin+"/"] = cdnScopeImports
	}
	for _, pkg := range resolvedPackages {
		url := cdnOrigin + "/"
		if len(pkg.Dependencies) > 0 || len(pkg.PeerDependencies) > 0 {
			url += "*" // externall deps marker
		}
		url += pkg.Name + "@" + pkg.Version
		importMap.Imports[pkg.Name] = url
		importMap.Imports[pkg.Name+"/"] = url + "/"
		if hasCdnScopedImports {
			delete(cdnScopeImports, pkg.Name)
			delete(cdnScopeImports, pkg.Name+"/")
		}
	}
	for _, pkg := range resolvedPackages {
		walkPackageDependencies(pkg, func(specifier, pkgName, pkgVersion, prefix string) {
			if _, ok := importMap.Imports[specifier]; !ok {
				prevUrl, prev := cdnScopeImports[specifier]
				deepCheck := true
			checkPrevUrl:
				if prev {
					pathname := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(prevUrl, cdnOrigin+"/"), "*"), "@")
					_, prevVersion := utils.SplitByFirstByte(pathname, '@')
					prevVersion, _ = utils.SplitByFirstByte(prevVersion, '/')
					if common.IsExactVersion(prevVersion) {
						if common.IsExactVersion(pkgVersion) {
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
					if scopeImports, ok := importMap.Scopes[cdnOrigin+"/"+pkg.Name+"@"+pkg.Version+"/"]; ok {
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
		return common.ImportMap{}, false
	}
	for _, pkg := range resolvedPackages {
		fmt.Println(term.Green("✔"), pkg.Name+term.Dim("@"+pkg.Version))
	}
	return importMap, true
}

func walkPackageDependencies(pkg PackageJSON, callback func(specifier, pkgName, pkgVersion, prefix string)) {
	if len(pkg.Dependencies) > 0 {
		_walkPackageDependencies(pkg.Dependencies, callback)
	}
	if len(pkg.PeerDependencies) > 0 {
		_walkPackageDependencies(pkg.PeerDependencies, callback)
	}
}

func _walkPackageDependencies(deps map[string]string, callback func(specifier, pkgName, pkgVersion, prefix string)) {
	for specifier, pkgVersion := range deps {
		pkgName := specifier
		pkg, err := common.ResolveDependencyVersion(pkgVersion)
		if err == nil && pkg.Name != "" {
			pkgName = pkg.Name
			pkgVersion = pkg.Version
		}
		var prefix string
		if pkg.Github {
			prefix = "/gh"
		} else if pkg.PkgPrNew {
			prefix = "/pr"
		}
		callback(specifier, pkgName, pkgVersion, prefix)
	}
}

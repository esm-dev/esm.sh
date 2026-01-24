package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/esm-dev/esm.sh/internal/importmap"
	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/ije/gox/term"
	"github.com/ije/gox/utils"
	"golang.org/x/net/html"
)

const addHelpMessage = `Add packages to the "importmap" in index.html

Usage: esm.sh add [...packages] [options]

Examples:
  esm.sh add react             ` + "\033[30m # latest \033[0m" + `
  esm.sh add react@19          ` + "\033[30m # semver range \033[0m" + `
  esm.sh add react@19.0.0      ` + "\033[30m # exact version \033[0m" + `

Arguments:
  ...packages    Packages to add

Options:
  --help, -h     Show help message
`

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <script type="importmap">
%s
  </script>
</head>
<body>
  <h1>Hello, world!</h1>
</body>
</html>
`

// Add adds packages to "importmap" script
func Add() {
	packages, help := parseCommandFlags()

	if help {
		fmt.Print(addHelpMessage)
		return
	}

	if len(packages) > 0 {
		err := updateImportMap(packages)
		if err != nil {
			fmt.Println(term.Red("✖︎"), "Failed to add packages: "+err.Error())
		}
	}
}

func updateImportMap(packages []string) (err error) {
	indexHtml, exists, err := lookupClosestFile("index.html")
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
					buf.WriteString("  <script type=\"importmap\">\n")
					var importMap importmap.ImportMap
					addImports(&importMap, packages)
					buf.WriteString(importMap.FormatJSON(2))
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
						buf.WriteString("<script type=\"importmap\">\n")
						var importMap importmap.ImportMap
						addImports(&importMap, packages)
						buf.WriteString(importMap.FormatJSON(2))
						buf.WriteString("\n  </script>\n  ")
						buf.Write(tokenizer.Raw())
						updated = true
						continue
					}
					if typeAttr == "importmap" && !updated {
						buf.Write(tokenizer.Raw())
						token := tokenizer.Next()
						var importMap importmap.ImportMap
						if token == html.TextToken {
							importMapRaw := bytes.TrimSpace(tokenizer.Text())
							if len(importMapRaw) > 0 {
								if json.Unmarshal(importMapRaw, &importMap) != nil {
									err = fmt.Errorf("invalid importmap script")
									return
								}
							}
						}
						buf.WriteString("\n")
						addImports(&importMap, packages)
						buf.WriteString(importMap.FormatJSON(2))
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
		var importMap importmap.ImportMap
		addImports(&importMap, packages)
		err = os.WriteFile(indexHtml, fmt.Appendf(nil, htmlTemplate, importMap.FormatJSON(2)), 0644)
		if err == nil {
			fmt.Println(term.Dim("Created index.html with importmap script."))
		}
	}
	return
}

func addImports(importMap *importmap.ImportMap, specifiers []string) {
	term.HideCursor()
	defer term.ShowCursor()

	var warnings []string
	var errors []error

	startTime := time.Now()
	spinner := term.NewSpinner(term.SpinnerConfig{})
	spinner.Start()
	var wg sync.WaitGroup
	var resovedImports []importmap.ImportMeta
	for _, specifier := range specifiers {
		wg.Go(func() {
			var scopeName string
			var pkgName string
			var subPath string
			var regPrefix string
			if strings.HasPrefix(specifier, "jsr:") {
				regPrefix = "jsr/"
				specifier = specifier[4:]
			} else if strings.ContainsRune(specifier, '/') && specifier[0] != '@' {
				regPrefix = "gh/"
				specifier = strings.Replace(specifier, "#", "@", 1) // owner/repo#branch -> owner/repo@branch
			}
			if len(specifier) > 0 && (specifier[0] == '@' || regPrefix == "gh/") {
				scopeName, specifier = utils.SplitByFirstByte(specifier, '/')
			}
			pkgName, subPath = utils.SplitByFirstByte(specifier, '/')
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
			meta, err := importmap.FetchImportMeta(importMap.CDNOrigin(), regPrefix, pkgName, version, subPath)
			if err != nil {
				errors = append(errors, err)
				return
			}
			resovedImports = append(resovedImports, meta)
		})
	}
	wg.Wait()

	var imports []importmap.ImportMeta
	for _, meta := range resovedImports {
		imports = append(imports, meta)
		if meta.SubPath == "" && len(meta.Exports) > 0 {
			// prompt
		}
	}

	var wg2 sync.WaitGroup
	for _, meta := range imports {
		wg2.Go(func() {
			warns, errs := importMap.AddImport(meta, false, nil)
			warnings = append(warnings, warns...)
			errors = append(errors, errs...)
		})
	}
	wg2.Wait()

	spinner.Stop()

	if len(errors) > 0 {
		for _, err := range errors {
			fmt.Println(term.Red("[error]"), err.Error())
		}
		return
	}

	record := make(map[string]string)
	for _, imp := range imports {
		specifier := imp.Specifier()
		record[specifier] = importMap.Imports[specifier]
	}
	keys := make([]string, 0, len(record))
	for key := range record {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		k := key
		if !strings.HasSuffix(k, "/") {
			k += " " // align with the next line
		}
		fmt.Println(term.Green("✔"), k, term.Dim("→"), term.Dim(record[key]))
	}

	if len(warnings) > 0 {
		for _, warning := range warnings {
			fmt.Println(term.Yellow("[warn]"), warning)
		}
	}

	fmt.Println(term.Green("✦"), "Done in", term.Dim(time.Since(startTime).String()))
}

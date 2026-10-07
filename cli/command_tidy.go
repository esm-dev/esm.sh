package cli

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/esm-dev/esm.sh/internal/importmap"
	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/ije/gox/term"
	"golang.org/x/net/html"
)

const tidyHelpMessage = `Clean up and optimize the "importmap" script in index.html

Usage: esm.sh tidy [options]

Options:
	--no-sri    No "integrity" attribute for the import map
  --help, -h  Show help message
`

// Tidy tidies up "importmap" script
func Tidy() {
	noSRI := flag.Bool("no-sri", false, "do not generate SRI for the import")
	_, help := parseCommandFlags()
	if help {
		fmt.Print(tidyHelpMessage)
		return
	}

	err := tidy(*noSRI)
	if err != nil {
		fmt.Fprintln(os.Stderr, term.Red("[error]"), "Failed to tidy up: "+err.Error())
		os.Exit(1)
	}
}

func tidy(noSRI bool) (err error) {
	indexHtml, exists, err := lookupClosestFile("index.html")
	if err != nil {
		err = fmt.Errorf("failed to lookup index.html: %w", err)
		return
	}

	if !exists {
		err = fmt.Errorf("index.html not found")
		return
	}

	f, err := os.Open(indexHtml)
	if err != nil {
		return
	}
	defer f.Close()

	tokenizer := html.NewTokenizer(f)
	buf := bytes.NewBuffer(nil)
	for {
		token := tokenizer.Next()
		if token == html.ErrorToken {
			if tokenizer.Err() != io.EOF {
				return tokenizer.Err()
			}
			break
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
				if typeAttr == "importmap" {
					buf.Write(tokenizer.Raw())
					var prevImportMap *importmap.ImportMap
					if tokenizer.Next() == html.TextToken {
						importMapJson := bytes.TrimSpace(tokenizer.Text())
						if len(importMapJson) > 0 {
							prevImportMap, err = importmap.Parse(nil, importMapJson)
							if err != nil {
								err = fmt.Errorf("invalid importmap script: %w", err)
								return
							}
						}
					}
					if prevImportMap == nil || prevImportMap.Imports.Len() == 0 {
						fmt.Println(term.Dim("No imports found."))
						return
					}
					buf.WriteString("\n")
					importMap := importmap.Blank()
					importMap.SetConfig(prevImportMap.Config())
					importMap.SetIntegrity(prevImportMap.Integrity())
					cdn := prevImportMap.Config().CDN
					if !strings.HasPrefix(cdn, "https://") && !strings.HasPrefix(cdn, "http://") {
						cdn = "https://esm.sh"
					}
					imports := make([]importmap.Import, 0, prevImportMap.Imports.Len())
					unmanagedCDN := false
					prevImportMap.Imports.Range(func(specifier string, url string) bool {
						if strings.HasPrefix(url, cdn+"/") {
							imp, err := importmap.ParseEsmPath(url)
							if err == nil && npm.IsExactVersion(imp.Version) && specifier == imp.Specifier(false) {
								imports = append(imports, imp)
								return true // continue
							}
							unmanagedCDN = true
						}
						importMap.Imports.Set(specifier, url)
						return true
					})
					prevImportMap.RangeScopes(func(scope string, imports *importmap.Imports) bool {
						if !unmanagedCDN && strings.HasPrefix(scope, cdn+"/") && strings.HasSuffix(scope, "/") {
							return true
						}
						importMap.SetScopeImports(scope, imports)
						return true
					})
					if len(imports) == 0 {
						fmt.Println(term.Dim("No imports found."))
						return
					}
					slices.SortFunc(imports, func(a, b importmap.Import) int {
						return strings.Compare(a.Specifier(true), b.Specifier(true))
					})
					if !addImports(importMap, imports, false, true, noSRI, "") {
						return fmt.Errorf("could not resolve imports")
					}
					buf.WriteString(importMap.FormatJSON(2))
					buf.WriteString("\n  ")
					continue
				}
			}
		}
		buf.Write(tokenizer.Raw())
	}
	fi, err := f.Stat()
	f.Close()
	if err != nil {
		return
	}
	err = os.WriteFile(indexHtml, buf.Bytes(), fi.Mode())
	return
}

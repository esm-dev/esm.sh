package cli

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/esm-dev/esm.sh/internal/importmap"
	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/ije/gox/term"
	"golang.org/x/net/html"
)

const tidyHelpMessage = `Clean up and optimize the "importmap" in index.html

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
		fmt.Println(term.Red("[error]"), "Failed to tidy up: "+err.Error())
	}
}

func tidy(noSRI bool) (err error) {
	indexHtml, exists, err := lookupClosestFile("index.html")
	if err != nil {
		err = fmt.Errorf("Failed to lookup index.html: %w", err)
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

	tokenizer := html.NewTokenizer(f)
	buf := bytes.NewBuffer(nil)
	for {
		token := tokenizer.Next()
		if token == html.ErrorToken && tokenizer.Err() == io.EOF {
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
					token := tokenizer.Next()
					var prevImportMap *importmap.ImportMap
					if token == html.TextToken {
						importMapJson := bytes.TrimSpace(tokenizer.Text())
						if len(importMapJson) > 0 {
							prevImportMap, err = importmap.Parse(nil, importMapJson)
							if err != nil {
								err = fmt.Errorf("invalid importmap script: %w", err)
								return
							}
						}
					}
					buf.WriteString("\n")
					importMap := importmap.Blank()
					importMap.SetConfig(prevImportMap.Config())
					imports := make([]importmap.Import, 0, prevImportMap.Imports.Len())
					prevImportMap.Imports.Range(func(specifier string, url string) bool {
						if strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://") {
							// todo: check hostname
							imp, err := importmap.ParseEsmPath(url)
							if err == nil {
								if npm.IsExactVersion(imp.Version) {
									imports = append(imports, imp)
								}
								return true // continue
							}
						}
						importMap.Imports.Set(specifier, url)
						return true
					})
					prevImportMap.RangeScopes(func(scope string, imports *importmap.Imports) bool {
						if strings.HasPrefix(scope, "https://") || strings.HasPrefix(scope, "http://") {
							// todo: check hostname
							if strings.HasSuffix(scope, "/") {
								return true // continue
							}
						}
						importMap.SetScopeImports(scope, imports)
						return true
					})
					specifiers := make([]string, 0, len(imports))
					for _, imp := range imports {
						specifiers = append(specifiers, imp.Specifier(true))
					}
					sort.Strings(specifiers)
					addImports(importMap, specifiers, false, false, noSRI)
					buf.WriteString(importMap.FormatJSON(2))
					buf.WriteString("\n  ")
					if token == html.EndTagToken {
						buf.Write(tokenizer.Raw())
					}
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

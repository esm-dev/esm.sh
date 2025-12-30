package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/esm-dev/esm.sh/internal/importmap"
	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/ije/gox/term"
	"golang.org/x/net/html"
)

const tidyHelpMessage = "\033[30mesm.sh - A nobuild tool for modern web development.\033[0m" + `

Usage: esm.sh tidy [options]

Options:
  --help, -h  Show help message
`

// Tidy tidies up "importmap" script
func Tidy() {
	help := flag.Bool("help", false, "Show help message")
	flag.Parse()

	if *help || strings.Contains(os.Args[1], "-h") {
		fmt.Print(tidyHelpMessage)
		return
	}

	fmt.Println(term.Dim("✦"), term.Dim("Tidying up..."))

	err := tidy()
	if err != nil {
		fmt.Println(term.Red("[error]"), "Failed to tidy up: "+err.Error())
	}

	fmt.Println(term.Green("✦"), "Tidied up successfully")
}

func tidy() (err error) {
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
					var prevImportMap importmap.ImportMap
					if token == html.TextToken {
						importMapRaw := bytes.TrimSpace(tokenizer.Text())
						if len(importMapRaw) > 0 {
							if json.Unmarshal(importMapRaw, &prevImportMap) != nil {
								err = fmt.Errorf("invalid importmap script")
								return
							}
						}
					}
					buf.WriteString("\n")
					importMap := importmap.ImportMap{
						Config:  prevImportMap.Config,
						Imports: map[string]string{},
						Scopes:  map[string]map[string]string{},
					}
					packages := make([]importmap.PackageInfo, 0, len(prevImportMap.Imports))
					for specifier, path := range prevImportMap.Imports {
						if strings.HasPrefix(path, "https://") || strings.HasPrefix(path, "http://") {
							// todo: check hostname
							pkgInfo, err := importmap.GetPackageInfoFromUrl(path)
							if err == nil {
								if npm.IsExactVersion(pkgInfo.Version) && !strings.HasSuffix(specifier, "/") {
									packages = append(packages, pkgInfo)
								}
								continue
							}
						}
						importMap.Imports[specifier] = path
					}
					for prefix, imports := range prevImportMap.Scopes {
						if strings.HasPrefix(prefix, "https://") || strings.HasPrefix(prefix, "http://") {
							// todo: check hostname
							if strings.HasSuffix(prefix, "/") {
								continue
							}
						}
						importMap.Scopes[prefix] = imports
					}
					packageNames := make([]string, 0, len(packages))
					for _, pkg := range packages {
						packageNames = append(packageNames, pkg.Name+"@"+pkg.Version)
					}
					addPackages(&importMap, packageNames)
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

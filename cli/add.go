package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/esm-dev/esm.sh/internal/importmap"
	"github.com/ije/gox/term"
	"golang.org/x/net/html"
)

const addHelpMessage = "\033[30mesm.sh - A nobuild tool for modern web development.\033[0m" + `

Usage: esm.sh add [...packages] [options]

Examples:
  esm.sh add react             ` + "\033[30m # latest \033[0m" + `
  esm.sh add react@19          ` + "\033[30m # semver range \033[0m" + `
  esm.sh add react@19.0.0      ` + "\033[30m # exact version \033[0m" + `

Arguments:
  [...packages]  Packages to add

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
	help := flag.Bool("help", false, "Show help message")
	arg0, argMore := parseCommandFlag(2)

	if *help || strings.Contains(os.Args[1], "-h") {
		fmt.Print(addHelpMessage)
		return
	}

	var packages []string
	if arg0 != "" {
		packages = append(packages, arg0)
		packages = append(packages, argMore...)
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
					addPackages(&importMap, packages)
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
						addPackages(&importMap, packages)
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
						addPackages(&importMap, packages)
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
		addPackages(&importMap, packages)
		err = os.WriteFile(indexHtml, fmt.Appendf(nil, htmlTemplate, importMap.FormatJSON(2)), 0644)
		if err == nil {
			fmt.Println(term.Dim("Created index.html with importmap script."))
		}
	}
	return
}

func addPackages(importMap *importmap.ImportMap, packages []string) {
	startTime := time.Now()
	spinner := term.NewSpinner("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏", 5)
	spinner.Start()
	addedPackages, warnings, errors := importMap.AddPackages(packages)
	spinner.Stop()

	if len(errors) > 0 {
		for _, err := range errors {
			fmt.Println(term.Red("[error]"), err.Error())
		}
		return
	}

	record := make(map[string]string)
	for _, pkg := range addedPackages {
		record[pkg.Name] = importMap.Imports[pkg.Name]
		record[pkg.Name+term.Dim("/*")] = importMap.Imports[pkg.Name+"/"]
	}
	keys := make([]string, 0, len(record))
	for key := range record {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Println(term.Green("✔"), key, term.Dim("→"), term.Dim(record[key]))
	}

	if len(warnings) > 0 {
		for _, warning := range warnings {
			fmt.Println(term.Yellow("[warn]"), warning)
		}
	}

	fmt.Println(term.Green("✦"), "Done in", term.Dim(time.Since(startTime).String()))
}

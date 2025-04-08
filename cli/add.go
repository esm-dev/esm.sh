package cli

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/esm-dev/esm.sh/internal/importmap"
	"github.com/goccy/go-json"
	"github.com/ije/gox/term"
	"golang.org/x/net/html"
)

const addHelpMessage = "\033[30mesm.sh - A nobuild tool for modern web development.\033[0m" + `

Usage: esm.sh add [...packages] [options]

Examples:
  esm.sh add react@19.0.0
  esm.sh add react@19 react-dom@19
  esm.sh add react react-dom @esm.sh/router

Arguments:
  [...packages]    Packages to add, separated by space

Options:
  --help           Show help message
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
					importMap := importmap.ImportMap{}
					if !importMap.AddPackages(packages) {
						return
					}
					imJson, _ := importMap.MarshalJSON()
					buf.Write(imJson)
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
						importMap := importmap.ImportMap{}
						if !importMap.AddPackages(packages) {
							return
						}
						imJson, _ := importMap.MarshalJSON()
						buf.Write(imJson)
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
						buf.WriteString("\n    ")
						if !importMap.AddPackages(packages) {
							return
						}
						imJson, _ := importMap.MarshalJSON()
						buf.Write(imJson)
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
		importMap := importmap.ImportMap{}
		if !importMap.AddPackages(packages) {
			return
		}
		imJson, _ := importMap.MarshalJSON()
		err = os.WriteFile(indexHtml, fmt.Appendf(nil, htmlTemplate, string(imJson)), 0644)
		if err == nil {
			fmt.Println(term.Dim("Created index.html with importmap script."))
		}
	}
	return
}

package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/esm-dev/esm.sh/internal/importmap"
	"github.com/ije/gox/set"
	"github.com/ije/gox/term"
	"golang.org/x/net/html"
)

const addHelpMessage = `Add imports to the "importmap" in index.html

Usage: esm.sh add [...imports] [options]

Examples:
  esm.sh add react             ` + "\033[30m # latest \033[0m" + `
  esm.sh add react@19          ` + "\033[30m # semver range \033[0m" + `
  esm.sh add react@19.0.0      ` + "\033[30m # exact version \033[0m" + `
  esm.sh add react/jsx-runtime ` + "\033[30m # sub-module \033[0m" + `
  esm.sh add react/            ` + "\033[30m # include all sub-modules \033[0m" + `

Arguments:
  ...imports     Imports to add

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

// Add adds imports to "importmap" script
func Add() {
	specifiers, help := parseCommandFlags()

	if help {
		fmt.Print(addHelpMessage)
		return
	}

	if len(specifiers) > 0 {
		err := updateImportMap(set.New(specifiers...).Values())
		if err != nil {
			fmt.Println(term.Red("✖︎"), "Failed to add packages: "+err.Error())
		}
	}
}

func updateImportMap(specifiers []string) (err error) {
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
					addImports(&importMap, specifiers, true)
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
						importMap := importmap.Blank()
						addImports(importMap, specifiers, true)
						buf.WriteString(importMap.FormatJSON(2))
						buf.WriteString("\n  </script>\n  ")
						buf.Write(tokenizer.Raw())
						updated = true
						continue
					}
					if typeAttr == "importmap" && !updated {
						buf.Write(tokenizer.Raw())
						token := tokenizer.Next()
						importMap := importmap.Blank()
						if token == html.TextToken {
							importMapRaw := bytes.TrimSpace(tokenizer.Text())
							if len(importMapRaw) > 0 {
								importMap, err = importmap.Parse(nil, importMapRaw)
								if err != nil {
									err = fmt.Errorf("invalid importmap script: %w", err)
									return
								}
							}
						}
						buf.WriteString("\n")
						addImports(importMap, specifiers, true)
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
		importMap := importmap.Blank()
		addImports(importMap, specifiers, true)
		err = os.WriteFile(indexHtml, fmt.Appendf(nil, htmlTemplate, importMap.FormatJSON(2)), 0644)
		if err == nil {
			fmt.Println(term.Dim("Created index.html with importmap script."))
		}
	}
	return
}

func addImports(importMap *importmap.ImportMap, specifiers []string, prompt bool) {
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
			meta, err := importMap.ParseImport(specifier)
			if err != nil {
				errors = append(errors, err)
				return
			}
			resovedImports = append(resovedImports, meta)
		})
	}
	wg.Wait()

	if len(errors) > 0 {
		for _, err := range errors {
			fmt.Println(term.Red("[error]"), err.Error())
		}
		spinner.Stop()
		return
	}

	var wg2 sync.WaitGroup
	for _, imp := range resovedImports {
		wg2.Go(func() {
			warns, errs := importMap.AddImport(imp, false, nil)
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

	for _, imp := range resovedImports {
		if imp.SubPath == "" && len(imp.Exports) > 0 && prompt {
			// prompt
			// selected := multiSelect(&termRaw{}, "Select the export to use", imp.Exports)
			// fmt.Println(selected)
		}
	}

	record := make(map[string]string)
	for _, imp := range resovedImports {
		specifier := imp.Specifier(false)
		record[specifier], _ = importMap.Imports.Get(specifier)
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

// Select asks the user to select an item from a list.
func multiSelect(raw term.Raw, prompt string, items []string) (selected []string) {
	fmt.Print(term.Cyan("? "))
	fmt.Println(prompt)

	defer func() {
		lines := len(items) + 1
		term.MoveCursorUp(lines)
		for range lines {
			term.ClearLine()
			os.Stdout.Write([]byte("\n")) // move to the next line
		}
		term.MoveCursorUp(lines)
	}()

	cursor := 0
	selectState := set.New[string]()
	printMultiSelectItems(items, selectState, cursor, false)

	for {
		key := raw.Next()
		switch key {
		case 3, 27: // Ctrl+C, Escape
			fmt.Print(term.Dim("Aborted."))
			fmt.Print("\n")
			os.Exit(0)
		case 13: // Enter
			selected = selectState.Values()
			return
		case 32: // Space
			item := items[cursor]
			if !selectState.Has(item) {
				selectState.Add(item)
			} else {
				selectState.Remove(item)
			}
			printMultiSelectItems(items, selectState, cursor, true)
		case 65, 16, 'p': // Up, ctrl+p, p
			if cursor > 0 {
				cursor--
				printMultiSelectItems(items, selectState, cursor, true)
			}
		case 66, 14, 'n': // Down, ctrl+n, n
			if cursor < len(items)-1 {
				cursor++
				printMultiSelectItems(items, selectState, cursor, true)
			}
		}
	}
}

func printMultiSelectItems(items []string, selectState *set.Set[string], cursor int, resetCursor bool) {
	if resetCursor {
		term.MoveCursorUp(len(items))
	}
	for i, name := range items {
		os.Stdout.Write([]byte("\r"))
		if selectState.Has(name) {
			os.Stdout.WriteString(term.Green("•"))
		} else {
			os.Stdout.WriteString(term.Dim("•"))
		}
		os.Stdout.Write([]byte(" "))
		if i == cursor {
			os.Stdout.WriteString(name)
		} else {
			os.Stdout.WriteString(term.Dim(name))
		}
		os.Stdout.Write([]byte("\n"))
	}
}

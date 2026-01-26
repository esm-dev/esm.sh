package cli

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/esm-dev/esm.sh/internal/importmap"
	"github.com/ije/gox/set"
	"github.com/ije/gox/term"
	"golang.org/x/net/html"
)

const addHelpMessage = `Add imports to the "importmap" in index.html

Usage: esm.sh add [options] [...imports]

Examples:
  esm.sh add react             ` + "\033[30m # use latest \033[0m" + `
  esm.sh add react@19          ` + "\033[30m # use semver range \033[0m" + `
  esm.sh add react@19.0.0      ` + "\033[30m # use exact version \033[0m" + `
  esm.sh add react/jsx-runtime ` + "\033[30m # specifiy a sub-module \033[0m" + `
  esm.sh add --all react       ` + "\033[30m # include all sub-modules of the import\033[0m" + `

Arguments:
  ...imports     Imports to add

Options:
	--all, -a      Add all modules of the import without prompt
	--no-prompt    Add imports without prompt
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
	<script type="module">
		import * as mod from "%s";
		console.log(mod);
	</script>
</head>
<body>
  <p>Build with <a href="https://esm.sh">esm.sh</a> 💚</p>
</body>
</html>
`

// Add adds imports to "importmap" script
func Add() {
	all := flag.Bool("all", false, "add all modules of the import")
	a := flag.Bool("a", false, "add all modules of the import")
	noPrompt := flag.Bool("no-prompt", false, "add imports without prompt")
	specifiers, help := parseCommandFlags()

	if help || len(specifiers) == 0 {
		fmt.Print(addHelpMessage)
		return
	}

	if len(specifiers) > 0 {
		err := updateImportMap(set.New(specifiers...).Values(), *all || *a, *noPrompt)
		if err != nil {
			fmt.Println(term.Red("✖︎"), "Failed to add packages: "+err.Error())
		}
	}
}

func updateImportMap(specifiers []string, all bool, noPrompt bool) (err error) {
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
					if addImports(&importMap, specifiers, !noPrompt, all) {
						buf.WriteString(importMap.FormatJSON(2))
						buf.WriteString("\n  </script>\n")
					}
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
						if addImports(importMap, specifiers, !noPrompt, all) {
							buf.WriteString(importMap.FormatJSON(2))
							buf.WriteString("\n  </script>\n  ")
						}
						buf.Write(tokenizer.Raw())
						updated = true
						continue
					}
					if typeAttr == "importmap" && !updated {
						buf.Write(tokenizer.Raw())
						token := tokenizer.Next()
						importMap := importmap.Blank()
						tagContent := tokenizer.Raw()
						if token == html.TextToken {
							importMapRaw := bytes.TrimSpace(tagContent)
							if len(importMapRaw) > 0 {
								importMap, err = importmap.Parse(nil, importMapRaw)
								if err != nil {
									err = fmt.Errorf("invalid importmap script: %w", err)
									return
								}
							}
						}
						if addImports(importMap, specifiers, !noPrompt, all) {
							buf.WriteString("\n")
							buf.WriteString(importMap.FormatJSON(2))
							buf.WriteString("\n  ")
						} else {
							buf.Write(tagContent)
						}
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
		if addImports(importMap, specifiers, !noPrompt, all) {
			err = os.WriteFile(indexHtml, fmt.Appendf(nil, htmlTemplate, importMap.FormatJSON(2), specifiers[0]), 0644)
			if err == nil {
				fmt.Println(term.Dim("Created index.html with importmap script."))
			}
		}
	}
	return
}

func addImports(im *importmap.ImportMap, specifiers []string, prompt bool, all bool) bool {
	// debug(skip term spinner and prompt)
	im.AddImportFromSpecifier(specifiers[0])
	return true

	term.HideCursor()
	defer term.ShowCursor()

	startTime := time.Now()
	spinner := term.NewSpinner(term.SpinnerConfig{})
	spinner.Start()

	// stop spinner and print errors
	onErrors := func(errors []error) {
		spinner.Stop()
		for _, err := range errors {
			fmt.Println(term.Red("[error]"), err.Error())
		}
	}

	var wg sync.WaitGroup
	var resolvedImports []importmap.ImportMeta
	var warnings []string
	var errors []error
	for _, specifier := range specifiers {
		wg.Go(func() {
			imp, err := im.ParseImport(specifier)
			if err != nil {
				errors = append(errors, err)
				return
			}
			resolvedImports = append(resolvedImports, imp)
		})
	}
	wg.Wait()

	var wg2 sync.WaitGroup
	if all && len(resolvedImports) > 0 {
		for _, imp := range resolvedImports {
			if len(imp.Exports) > 0 {
				for _, exportPath := range imp.Exports {
					if strings.HasPrefix(exportPath, "./") && !strings.HasSuffix(exportPath, ".css") && !strings.HasSuffix(exportPath, ".json") && !strings.ContainsRune(exportPath, '*') {
						wg2.Go(func() {
							meta, err := im.FetchImportMeta(importmap.Import{
								Name:    imp.Name,
								Version: imp.Version,
								SubPath: exportPath[2:],
								Github:  imp.Github,
								Jsr:     imp.Jsr,
							})
							if err != nil {
								errors = append(errors, err)
								return
							}
							resolvedImports = append(resolvedImports, meta)
						})
					}
				}
			}
		}
	}
	wg2.Wait()

	if len(errors) > 0 {
		onErrors(errors)
		return false
	}

	for _, imp := range resolvedImports {
		warns, errors := im.AddImport(imp)
		if len(errors) > 0 {
			onErrors(errors)
			return false
		}
		warnings = append(warnings, warns...)
	}

	spinner.Stop()

	for _, imp := range resolvedImports {
		if imp.SubPath == "" && len(imp.Exports) > 0 && prompt {
			// prompt
			// selected := multiSelect(&termRaw{}, "Select the export to use", imp.Exports)
			// fmt.Println(selected)
		}
	}

	record := make(map[string]string)
	for _, imp := range resolvedImports {
		specifier := imp.Specifier(false)
		record[specifier], _ = im.Imports.Get(specifier)
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
	return true
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

package cli

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
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
	--all, -a      Add all sub-modules of the import without prompt
	--no-sri       No "integrity" attribute added
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
	noSRI := flag.Bool("no-sri", false, "do not generate SRI for the import")
	specifiers, help := parseCommandFlags()

	if help || len(specifiers) == 0 {
		fmt.Print(addHelpMessage)
		return
	}

	err := updateImportMap(set.New(specifiers...).Values(), *all || *a, *noPrompt, *noSRI)
	if err != nil {
		fmt.Println(term.Red("✖︎"), "Failed to add packages: "+err.Error())
	}
}

func updateImportMap(specifiers []string, all bool, noPrompt bool, noSRI bool) (err error) {
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
					if addImports(&importMap, specifiers, !noPrompt, all, noSRI) {
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
						if addImports(importMap, specifiers, !noPrompt, all, noSRI) {
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
						if addImports(importMap, specifiers, !noPrompt, all, noSRI) {
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
		if addImports(importMap, specifiers, !noPrompt, all, noSRI) {
			err = os.WriteFile(indexHtml, fmt.Appendf(nil, htmlTemplate, importMap.FormatJSON(2), specifiers[0]), 0644)
			if err == nil {
				fmt.Println(term.Dim("Created index.html with importmap script."))
			}
		}
	}
	return
}

func addImports(im *importmap.ImportMap, specifiers []string, prompt bool, all bool, noSRI bool) bool {
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

	if len(errors) > 0 {
		onErrors(errors)
		return false
	}

	if all {
		var wg sync.WaitGroup
		for _, imp := range resolvedImports {
			if len(imp.Exports) > 0 {
				for _, exportPath := range imp.Exports {
					if validateExportPath(exportPath) {
						wg.Go(func() {
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
		wg.Wait()
		if len(errors) > 0 {
			onErrors(errors)
			return false
		}
	}

	for _, imp := range resolvedImports {
		warns, errors := im.AddImport(imp, noSRI)
		if len(errors) > 0 {
			onErrors(errors)
			return false
		}
		warnings = append(warnings, warns...)
	}

	spinner.Stop()

	if prompt {
		term := &termRaw{}
		if term.isTTY() {
			for _, imp := range resolvedImports {
				if imp.SubPath == "" && len(imp.Exports) > 0 {
					var subModules = make([]string, 0, len(imp.Exports))
					for _, exportPath := range imp.Exports {
						if validateExportPath(exportPath) {
							subModules = append(subModules, exportPath[2:])
						}
					}
					if len(subModules) > 0 {
						ui := &subModuleSelectUI{term: term, im: im, mainImport: &imp}
						ui.init(subModules)
						if ui.termHeight >= 3 {
							ui.show()
						}
					}
				}
			}
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

type subModuleSelectUI struct {
	term         *termRaw
	im           *importmap.ImportMap
	mainImport   *importmap.ImportMeta
	noSRI        bool
	cursor       int
	subModules   []string
	state        []uint8 // 0 - not added, 1 - added, 2 - loading, 3 - error
	spinnerIndex int
	spinnerTimer *time.Timer
	spinnerChars []string
	termHeight   int
}

func (ui *subModuleSelectUI) init(subModules []string) {
	ui.subModules = subModules
	ui.state = make([]uint8, len(subModules))
	ui.spinnerChars = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	_, height, err := ui.term.GetSize()
	if err == nil {
		ui.termHeight = height
	}
}

func (ui *subModuleSelectUI) show() {
	fmt.Println(term.Cyan("Select sub-modules of " + ui.mainImport.Specifier(true) + ":"))
	ui.render(false)

	defer func() {
		ui.clearLines()
		if ui.spinnerTimer != nil {
			ui.spinnerTimer.Stop()
			ui.spinnerTimer = nil
		}
	}()

	for {
		key := ui.term.Next()
		switch key {
		case 3, 27: // Ctrl+C, Escape
			ui.clearLines()
			os.Stdout.WriteString(term.Dim("Aborted.\n"))
			term.ShowCursor()
			os.Exit(0)
			return
		case 13: // Enter
			if !ui.isPending() {
				return
			}
		case 32: // Space
			state := ui.state[ui.cursor]
			switch state {
			case 0, 3:
				// add
				cur := ui.cursor
				ui.state[cur] = 2
				ui.startSpinner()
				go func() {
					errors, _ := ui.im.AddImportFromSpecifier(ui.mainImport.Specifier(true)+"/"+ui.subModules[cur], ui.noSRI)
					if len(errors) > 0 {
						ui.state[cur] = 3
					} else {
						ui.state[cur] = 1
					}
				}()
			case 1:
				// remove
				ui.im.Imports.Delete(ui.mainImport.Specifier(false) + "/" + ui.subModules[ui.cursor])
				ui.state[ui.cursor] = 0
			}
			ui.render(true)
		case 65, 16, 'p': // Up, ctrl+p, p
			if ui.cursor > 0 {
				ui.cursor--
				ui.render(true)
			}
		case 66, 14, 'n': // Down, ctrl+n, n
			if ui.cursor < len(ui.subModules)-1 {
				ui.cursor++
				ui.render(true)
			}
		}
	}
}

func (ui *subModuleSelectUI) isPending() bool {
	return slices.Contains(ui.state, 2)
}

func (ui *subModuleSelectUI) clearLines() {
	func() {
		height := ui.maxLines()
		term.MoveCursorUp(height)
		for range height {
			term.ClearLine()
			os.Stdout.Write([]byte{'\n'}) // move to the next line
		}
		term.MoveCursorUp(height)
	}()
}

func (ui *subModuleSelectUI) maxLines() int {
	return min(ui.termHeight-2, len(ui.subModules))
}

func (ui *subModuleSelectUI) render(resetCursor bool) {
	visibleOptions := ui.subModules[:ui.maxLines()]
	if resetCursor {
		term.MoveCursorUp(len(visibleOptions) - 1)
	}
	for i, exportPath := range visibleOptions {
		state := ui.state[i]
		os.Stdout.WriteString("\r")
		term.ClearLineRight()
		switch state {
		case 0:
			if i == ui.cursor {
				os.Stdout.WriteString("○")
			} else {
				os.Stdout.WriteString(term.Dim("○"))
			}
		case 1:
			os.Stdout.WriteString(term.Green("✔"))
		case 2:
			os.Stdout.WriteString(term.Dim(ui.spinnerChars[ui.spinnerIndex]))
		case 3:
			os.Stdout.WriteString(term.Red("✖︎"))
		}
		os.Stdout.Write([]byte{' '})
		specifier := ui.mainImport.Specifier(false) + "/" + exportPath
		if i == ui.cursor {
			os.Stdout.WriteString(term.Bold(specifier))
			switch state {
			case 0:
				os.Stdout.WriteString(term.Dim(" space to add, enter to confirm"))
			case 1:
				os.Stdout.WriteString(term.Dim(" space to remove, enter to confirm"))
			case 2:
				os.Stdout.WriteString(term.Dim(" ..."))
			case 3:
				os.Stdout.WriteString(term.Dim(" error"))
			}
		} else {
			os.Stdout.WriteString(term.Dim(specifier))
			if state == 3 {
				os.Stdout.WriteString(term.Dim(" error"))
			}
		}
		if i < len(visibleOptions)-1 {
			os.Stdout.Write([]byte{'\n'})
		}
	}
}

func (ui *subModuleSelectUI) startSpinner() {
	if ui.spinnerTimer != nil {
		ui.spinnerTimer.Stop()
	}
	fps := 5
	ui.spinnerTimer = time.AfterFunc(time.Second/time.Duration(fps), ui.startSpinner)
	ui.spinnerIndex++
	if ui.spinnerIndex >= len(ui.spinnerChars) {
		ui.spinnerIndex = 0
	}
	ui.render(true)
}

func validateExportPath(exportPath string) bool {
	return strings.HasPrefix(exportPath, "./") && !strings.HasSuffix(exportPath, ".css") && !strings.HasSuffix(exportPath, ".json") && !strings.ContainsRune(exportPath, '*')
}

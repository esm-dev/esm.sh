package cli

import (
	"bytes"
	"embed"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ije/gox/term"
)

//go:embed demo
var efs embed.FS

const initHelpMessage = "\033[30mesm.sh - A nobuild tool for modern web development.\033[0m" + `

Usage: esm.sh init [project-name] [options]

Arguments:
  [project-name]     Name of the project, default is "esm-app"

Options:
  --framework        JavaScript framework, Available options: Vanilla, React, Preact, Vue, Svelte
  --css-framework    CSS framework, Available options: Vanilla, UnoCSS
  --typescript       Use TypeScript, default is false
  --help             Show help message
`

var frameworks = []string{
	"Vanilla",
	"React",
	"Preact",
	"Vue",
	"Svelte",
}

var cssFrameworks = []string{
	"Vanilla",
	"UnoCSS",
}

// Create a new nobuild web app with esm.sh CDN.
func Init() {
	framework := flag.String("framework", "", "JavaScript framework")
	cssFramework := flag.String("css-framework", "", "CSS framework")
	typescript := flag.Bool("typescript", false, "Use TypeScript")
	help := flag.Bool("help", false, "Show help message")
	projectName, _ := parseCommandFlag(2)
	raw := &termRaw{}

	if *help {
		fmt.Print(initHelpMessage)
		return
	}

	if projectName == "" {
		projectName = term.Input(raw, "Project name:", "esm-app")
	}

	if *framework == "" {
		*framework = term.Select(raw, "Select a framework:", frameworks)
	} else if !slices.Contains(frameworks, *framework) {
		fmt.Println("Invalid framework: ", *framework)
		os.Exit(1)
	}

	if *cssFramework == "" {
		*cssFramework = term.Select(raw, "Select a CSS framework:", cssFrameworks)
	} else if !slices.Contains(cssFrameworks, *cssFramework) {
		*cssFramework = cssFrameworks[0]
	}

	if !slices.Contains(os.Args, "--typescript") {
		*typescript = term.Select(raw, "Select a variant:", []string{"JavaScript", "TypeScript"}) == "TypeScript"
	}

	_, err := os.Lstat(projectName)
	if err == nil || os.IsExist(err) {
		if !term.Confirm(raw, "The directory already exists, do you want to overwrite it?") {
			fmt.Println(term.Dim("Canceled."))
			return
		}
	}

	dir := "demo/" + strings.ToLower(*framework)
	if *cssFramework == "UnoCSS" {
		dir = "demo/with-unocss/" + strings.ToLower(*framework)
	}
	err = walkEmbedFS(&efs, dir, func(filename string) error {
		savePath := projectName + strings.TrimPrefix(filename, dir)
		os.MkdirAll(filepath.Dir(savePath), 0755)
		if !*typescript {
			if (strings.HasSuffix(savePath, ".ts") || strings.HasSuffix(savePath, ".tsx")) && !strings.HasSuffix(savePath, ".d.ts") {
				data, err := efs.ReadFile(filename)
				if err != nil {
					return err
				}
				if strings.HasSuffix(savePath, ".tsx") {
					savePath = strings.TrimSuffix(savePath, ".tsx") + ".jsx"
				} else {
					savePath = strings.TrimSuffix(savePath, ".ts") + ".js"
				}
				data = bytes.ReplaceAll(data, []byte(")!"), []byte(")"))
				data = bytes.ReplaceAll(data, []byte(".ts\""), []byte(".js\""))
				data = bytes.ReplaceAll(data, []byte(".tsx\""), []byte(".jsx\""))
				return os.WriteFile(savePath, data, 0644)
			} else if strings.HasSuffix(savePath, ".html") {
				data, err := efs.ReadFile(filename)
				if err != nil {
					return err
				}
				data = bytes.ReplaceAll(data, []byte(".ts\""), []byte(".js\""))
				data = bytes.ReplaceAll(data, []byte(".tsx\""), []byte(".jsx\""))
				return os.WriteFile(savePath, data, 0644)
			} else if strings.HasSuffix(savePath, ".vue") || strings.HasSuffix(savePath, ".svelte") {
				data, err := efs.ReadFile(filename)
				if err != nil {
					return err
				}
				data = bytes.ReplaceAll(data, []byte(" lang=\"ts\""), []byte(""))
				return os.WriteFile(savePath, data, 0644)
			}
		}
		f, err := efs.Open(filename)
		if err != nil {
			return err
		}
		defer f.Close()
		w, err := os.Create(savePath)
		if err != nil {
			return err
		}
		defer w.Close()
		_, err = io.Copy(w, f)
		return err
	})
	if err != nil {
		fmt.Println(term.Red("✘ Failed to create project: " + err.Error()))
		os.Exit(1)
	}

	fmt.Println(" ")
	fmt.Println(term.Dim("Project created successfully."))
	fmt.Println(term.Dim("We highly recommend installing our VS Code extension for a better DX: https://link.esm.sh/vsce"))
	fmt.Println(term.Dim("To start the app in development mode, run:"))
	fmt.Println(" ")
	fmt.Println(term.Dim("$ ") + "cd " + projectName)
	if strings.Contains(os.Args[0], "/node_modules/") {
		fmt.Println(term.Dim("$ ") + "npx esm.sh dev")
	} else {
		fmt.Println(term.Dim("$ ") + "esm.sh dev")
	}
	fmt.Println(" ")
}

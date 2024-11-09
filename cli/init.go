package cli

import (
	"bytes"
	"embed"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ije/gox/term"
	xterm "golang.org/x/term"
)

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

var langVariants = []string{
	"JavaScript",
	"TypeScript",
}

// Create a new esm.sh web app
func Init(efs *embed.FS) {
	framework := flag.String("framework", "", "javascript framework")
	cssFramework := flag.String("css-framework", "", "CSS framework")
	lang := flag.String("lang", "", "language")
	args := parseCommandFlag()

	projectName := ""
	if len(args) > 0 {
		projectName = args[0]
	}
	if projectName == "" {
		projectName = termInput("Project name:", "esm-app")
	}

	if *framework == "" {
		*framework = termSelect("Select a framework:", frameworks)
	} else if !includes(frameworks, *framework) {
		fmt.Println("Invalid framework: ", *framework)
		os.Exit(1)
	}

	if *cssFramework == "" {
		*cssFramework = termSelect("Select a CSS framework:", cssFrameworks)
	} else if !includes(cssFrameworks, *cssFramework) {
		*cssFramework = cssFrameworks[0]
	}

	if *lang == "" {
		*lang = termSelect("Select a variant:", langVariants)
	} else if !includes(langVariants, *lang) {
		*lang = langVariants[0]
	}

	_, err := os.Lstat(projectName)
	if err == nil || os.IsExist(err) {
		if !termConfirm("The directory already exists, do you want to overwrite it?") {
			fmt.Println(term.Dim("Canceled."))
			return
		}
	}

	dir := "demo/" + strings.ToLower(*framework)
	if *cssFramework == "UnoCSS" {
		dir = "demo/with-unocss/" + strings.ToLower(*framework)
	}
	err = walkEFS(efs, dir, func(filename string) error {
		savePath := projectName + strings.TrimPrefix(filename, dir)
		os.MkdirAll(filepath.Dir(savePath), 0755)
		if *lang == "JavaScript" {
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
	fmt.Println(term.Dim("To start the development server:"))
	fmt.Println(" ")
	fmt.Println(term.Dim("$ ") + "cd " + projectName + " && esm.sh run")
	fmt.Println(" ")
}

func walkEFS(efs *embed.FS, dir string, cb func(filename string) error) error {
	entries, err := efs.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			err = walkEFS(efs, dir+"/"+entry.Name(), cb)
			if err != nil {
				return err
			}
		} else {
			err = cb(dir + "/" + entry.Name())
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func termConfirm(propmt string) (value bool) {
	fmt.Print(term.Cyan("? "))
	fmt.Print(propmt + " ")
	fmt.Print(term.Dim("(y/N)"))
	defer func() {
		term.ClearLine()
		fmt.Print("\r")
	}()
	for {
		key := getRawInput()
		switch key {
		case 3, 27: // Ctrl+C, Escape
			fmt.Print("\n")
			fmt.Print(term.Dim("Aborted."))
			fmt.Print("\n")
			os.Exit(0)
		case 13, 32: // Enter, Space
			return false
		case 'y':
			return true
		case 'n':
			return false
		}
	}
}

func termInput(propmt string, defaultValue string) (value string) {
	fmt.Print(term.Cyan("? "))
	fmt.Print(propmt + " ")
	fmt.Print(term.Dim(defaultValue))
	buf := make([]byte, 1024)
	bufN := 0
loop:
	for {
		key := getRawInput()
		switch key {
		case 3, 27: // Ctrl+C, Escape
			fmt.Print("\n")
			fmt.Print(term.Dim("Aborted."))
			fmt.Print("\n")
			os.Exit(0)
		case 13, 32: // Enter, Space
			break loop
		case 127, 8: // Backspace, Ctrl+H
			if bufN > 0 {
				bufN--
				fmt.Print("\b \b")
			} else {
				term.ClearLine()
				fmt.Print("\r")
				fmt.Print(term.Cyan("? "))
				fmt.Print(propmt + " ")
			}
		default:
			if (key >= 'a' && key <= 'z') || (key >= '0' && key <= '9') || key == '-' {
				if bufN == 0 {
					term.ClearLine()
					fmt.Print("\r")
					fmt.Print(term.Cyan("? "))
					fmt.Print(propmt + " ")
				}
				buf[bufN] = key
				bufN++
				fmt.Print(string(key))
			}
		}
	}
	if bufN > 0 {
		value = string(buf[:bufN])
	} else {
		value = defaultValue
	}
	fmt.Print("\r")
	fmt.Print(term.Green("✔ "))
	fmt.Print(propmt + " ")
	fmt.Print(term.Dim(value))
	fmt.Print("\n")
	return
}

func termSelect(propmt string, items []string) (selected string) {
	fmt.Print(term.Cyan("? "))
	fmt.Println(propmt)
	current := 0
	printSelectUI(items, current)

	defer func() {
		term.MoveCursorUp(len(items) + 1)
		fmt.Print(term.Green("✔ "))
		fmt.Print(propmt + " ")
		fmt.Print(term.Dim(selected))
		fmt.Print("\n")
		for i := 0; i < len(items); i++ {
			term.ClearLine()
			fmt.Print("\n")
		}
		term.MoveCursorUp(len(items))
	}()

	term.HideCursor()
	defer term.ShowCursor()

	for {
		key := getRawInput()
		switch key {
		case 3, 27: // Ctrl+C, Escape
			fmt.Print(term.Dim("Aborted."))
			fmt.Print("\n")
			term.ShowCursor()
			os.Exit(0)
		case 13, 32: // Enter, Space
			selected = items[current]
			return
		case 65, 16, 'p': // Up, ctrl+p, p
			if current > 0 {
				term.MoveCursorUp(len(items))
				current--
				printSelectUI(items, current)
			}
		case 66, 14, 'n': // Down, ctrl+n, n
			if current < len(items)-1 {
				term.MoveCursorUp(len(items))
				current++
				printSelectUI(items, current)
			}
		}
	}
}

func printSelectUI(items []string, selected int) {
	for i, name := range items {
		if i == selected {
			fmt.Println("\r> ", name)
		} else {
			fmt.Println("\r  ", term.Dim(name))
		}
	}
}

// Read raw input from the terminal.
func getRawInput() byte {
	oldState, err := xterm.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}
	defer xterm.Restore(int(os.Stdin.Fd()), oldState)

	buf := make([]byte, 3)
	n, err := os.Stdin.Read(buf)
	if err != nil {
		panic(err)
	}

	// The third byte is the key specific value we are looking for.
	// See: https://en.wikipedia.org/wiki/ANSI_escape_code
	if n == 3 {
		return buf[2]
	}

	return buf[0]
}

func includes(arr []string, value string) bool {
	for _, v := range arr {
		if v == value {
			return true
		}
	}
	return false
}

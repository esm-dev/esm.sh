package cli

import (
	"fmt"
	"os"

	"github.com/ije/gox/term"
	xterm "golang.org/x/term"
)

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

	current := 0
	printSelectItems(items, current)

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
				current--
				term.MoveCursorUp(len(items))
				printSelectItems(items, current)
			}
		case 66, 14, 'n': // Down, ctrl+n, n
			if current < len(items)-1 {
				current++
				term.MoveCursorUp(len(items))
				printSelectItems(items, current)
			}
		}
	}
}

func printSelectItems(items []string, selected int) {
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

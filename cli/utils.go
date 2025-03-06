package cli

import (
	"flag"
	"os"
	"strings"

	"golang.org/x/term"
)

// termRaw implements the github.com/ije/gox/term.Raw interface.
type termRaw struct{}

func (t *termRaw) Next() byte {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

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

// parseCommandFlag parses the command flag.
func parseCommandFlag() (string, []string) {
	flag.CommandLine.Parse(os.Args[2:])

	args := make([]string, 0, len(os.Args)-2)
	nextVaule := false
	for _, arg := range os.Args[2:] {
		if !strings.HasPrefix(arg, "-") {
			if !nextVaule {
				args = append(args, arg)
			} else {
				nextVaule = false
			}
		} else if !strings.Contains(arg, "=") {
			nextVaule = true
		}
	}
	if len(args) == 0 {
		return "", nil
	}
	return args[0], args[1:]
}

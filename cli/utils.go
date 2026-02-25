package cli

import (
	"flag"
	"os"
	"path/filepath"

	"golang.org/x/term"
)

// termRaw implements the `term.Raw“ interface of github.com/ije/gox
type termRaw struct {
	fd int
}

func newTermRaw() *termRaw {
	return &termRaw{
		fd: int(os.Stdin.Fd()),
	}
}

func (t *termRaw) Next() byte {
	oldState, err := term.MakeRaw(t.fd)
	if err != nil {
		panic(err)
	}
	defer term.Restore(t.fd, oldState)

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

func (t *termRaw) GetSize() (width int, height int, err error) {
	return term.GetSize(t.fd)
}

func (t *termRaw) isTTY() bool {
	return term.IsTerminal(t.fd)
}

// parseCommandFlags parses the command flags
func parseCommandFlags() (args []string, helpFlag bool) {
	help := flag.Bool("help", false, "Print help message")
	h := flag.Bool("h", false, "Print help message")
	flag.CommandLine.Parse(os.Args[2:])
	return flag.CommandLine.Args(), *help || *h
}

func lookupClosestFile(name string) (filename string, exists bool, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false, err
	}
	dir := cwd
	for {
		indexHtml := filepath.Join(dir, name)
		fi, err := os.Stat(indexHtml)
		if err == nil && !fi.IsDir() {
			return indexHtml, true, nil
		}
		if err != nil && os.IsExist(err) {
			return "", false, err
		}
		dir = filepath.Dir(dir)
		if dir == "/" || (os.PathSeparator == '\\' && len(dir) <= 3) {
			break
		}
	}
	return filepath.Join(cwd, name), false, nil
}

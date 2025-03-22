package cli

import (
	"embed"
	"flag"
	"os"
	"path/filepath"
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

// parseCommandFlag parses the command flag
func parseCommandFlag(start int) (string, []string) {
	if start >= len(os.Args) {
		start = len(os.Args)
	}
	flag.CommandLine.Parse(os.Args[start:])
	args := make([]string, 0, len(os.Args)-2)
	nextVaule := false
	for _, arg := range os.Args[start:] {
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

func lookupCloestFile(basename string) (filename string, exists bool, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false, err
	}
	dir := cwd
	for {
		indexHtml := filepath.Join(dir, basename)
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
	return filepath.Join(cwd, basename), false, nil
}

func walkEmbedFS(fs *embed.FS, dir string, callback func(filename string) error) error {
	entries, err := efs.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			err = walkEmbedFS(fs, dir+"/"+entry.Name(), callback)
			if err != nil {
				return err
			}
		} else {
			err = callback(dir + "/" + entry.Name())
			if err != nil {
				return err
			}
		}
	}
	return nil
}

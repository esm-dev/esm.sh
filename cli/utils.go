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

// parseCommandFlags parses the command flags
func parseCommandFlags() (args []string, helpFlag bool) {
	rawArgs := make([]string, 0, len(os.Args)-2)
	for _, arg := range os.Args[2:] {
		if arg == "-h" || arg == "--help" {
			helpFlag = true
		} else {
			rawArgs = append(rawArgs, arg)
		}
	}
	flag.CommandLine.Parse(rawArgs)
	args = make([]string, 0, len(rawArgs))
	nextVaule := false
	for _, arg := range rawArgs {
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
	return
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

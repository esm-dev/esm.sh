package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/ije/gox/utils"
)

func main() {
	root, _ := filepath.Abs(os.Args[1] + "/..")
	readme, err := ioutil.ReadFile(path.Join(root, "README.md"))
	if err != nil {
		fmt.Println(err)
		return
	}
	err = ioutil.WriteFile(path.Join(root, "server", "readme_md.go"), []byte(strings.Join([]string{
		"package server",
		"func init() {",
		"    readmemd = " + strings.TrimSpace(string(utils.MustEncodeJSON(string(readme)))),
		"}",
	}, "\n")), 0644)
	if err != nil {
		fmt.Println(err)
		return
	}
}

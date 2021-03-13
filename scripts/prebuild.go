package main

import (
	"encoding/base64"
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
	err = ioutil.WriteFile(path.Join(root, "server", "auto_readme.go"), []byte(strings.Join([]string{
		"package server",
		"func init() {",
		"    readme = " + strings.TrimSpace(string(utils.MustEncodeJSON(string(readme)))),
		"}",
	}, "\n")), 0644)
	if err != nil {
		fmt.Println(err)
		return
	}

	entries, err := ioutil.ReadDir(path.Join(root, "polyfills"))
	if err != nil {
		fmt.Println(err)
		return
	}
	polyfills := map[string]string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			data, err := ioutil.ReadFile(path.Join(root, "polyfills", entry.Name()))
			if err != nil {
				fmt.Println(err)
				return
			}
			if err == nil {
				polyfills[entry.Name()] = string(data)
			}
		}
	}
	err = ioutil.WriteFile(path.Join(root, "server", "auto_polyfills.go"), []byte(strings.Join([]string{
		"package server",
		"func init() {",
		"    polyfills = map[string]string" + strings.TrimSpace(string(utils.MustEncodeJSON(polyfills))),
		"}",
	}, "\n")), 0644)
	if err != nil {
		fmt.Println(err)
		return
	}

	mmdata, err := ioutil.ReadFile(path.Join(root, "third_party/china_ip_list/china_ip_list.mmdb"))
	if err != nil {
		fmt.Println(err)
		return
	}
	mmdatastring := base64.StdEncoding.EncodeToString(mmdata)
	err = ioutil.WriteFile(path.Join(root, "server", "auto_mmdbr.go"), []byte(strings.Join([]string{
		"package server",
		`import "encoding/base64"`,
		`import "github.com/oschwald/maxminddb-golang"`,
		"func init() {",
		"    mmdataRaw := " + strings.TrimSpace(string(utils.MustEncodeJSON(mmdatastring))),
		"    mmdata, err := base64.StdEncoding.DecodeString(mmdataRaw)",
		"    if err != nil {",
		"        panic(err)",
		"    }",
		"    mmdbr, err = maxminddb.FromBytes(mmdata)",
		"    if err != nil {",
		"        panic(err)",
		"    }",
		"}",
	}, "\n")), 0644)
	if err != nil {
		fmt.Println(err)
		return
	}
}

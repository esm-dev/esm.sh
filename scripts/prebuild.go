package main

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/ije/gox/utils"
)

const mmdb_china_ip_list_tag = "20210308"

var httpClient = &http.Client{
	Transport: &http.Transport{
		Dial: func(network, addr string) (conn net.Conn, err error) {
			conn, err = net.DialTimeout(network, addr, 15*time.Second)
			if err != nil {
				return conn, err
			}

			// Set a one-time deadline for potential SSL handshaking
			conn.SetDeadline(time.Now().Add(60 * time.Second))
			return conn, nil
		},
		MaxIdleConnsPerHost:   5,
		ResponseHeaderTimeout: 60 * time.Second,
	},
}

func main() {
	root, err := filepath.Abs(os.Args[1] + "/..")
	if err != nil {
		fmt.Println(err)
		return
	}

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

	if len(os.Args) < 3 || os.Args[2] != "--china-ip" {
		return
	}

	saveFilename := path.Join(root, ".dev", fmt.Sprintf("china_ip_list_%s.mmdb", mmdb_china_ip_list_tag))

	var mmdata []byte
	if fi, e := os.Lstat(saveFilename); e == nil && !fi.IsDir() {
		mmdata, err = ioutil.ReadFile(saveFilename)
	} else {
		dlUrl := fmt.Sprintf("https://github.com/alecthw/mmdb_china_ip_list/releases/download/%s/china_ip_list.mmdb", mmdb_china_ip_list_tag)
		fmt.Printf("Download %s\n", dlUrl)
		resp, err := httpClient.Get(dlUrl)
		if err != nil {
			fmt.Println(err)
			return
		}
		mmdata, err = ioutil.ReadAll(resp.Body)
		if err == nil {
			os.MkdirAll(path.Join(root, ".dev"), 0755)
			ioutil.WriteFile(saveFilename, mmdata, 0644)
		}
	}
	if err != nil {
		fmt.Println(err)
		return
	}

	mmdataString := base64.StdEncoding.EncodeToString(mmdata)
	err = ioutil.WriteFile(path.Join(root, "server", "auto_mmdbr.go"), []byte(strings.Join([]string{
		"package server",
		`import "encoding/base64"`,
		`import "github.com/oschwald/maxminddb-golang"`,
		"func init() {",
		"    mmdataRaw := " + strings.TrimSpace(string(utils.MustEncodeJSON(mmdataString))),
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

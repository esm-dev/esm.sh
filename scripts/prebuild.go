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

	var mmdata []byte
	saveFilename := path.Join(root, ".dev", fmt.Sprintf("china_ip_list_%s.mmdb", mmdb_china_ip_list_tag))
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
	err = ioutil.WriteFile(path.Join(root, "server", "mmdb_china_ip_list.go"), []byte(strings.Join([]string{
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

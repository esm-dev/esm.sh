package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/ije/gox/cache"
	"github.com/ije/gox/utils"
)

const (
	minNodejsVersion = 12
	nodejsLatestLTS  = "12.18.4"
)

// NodejsVersion defines nodejs version
type NodejsVersion struct {
	Node string `json:"node"`
	Npm  string `json:"npm"`
	Yarn string `json:"-"`
}

// NodeEnv defines the nodejs env
type NodeEnv struct {
	cache    cache.Cache
	version  NodejsVersion
	registry string
}

func checkNodeEnv() (env *NodeEnv, err error) {
	cache, err := cache.New("memory")
	if err != nil {
		return
	}
	env = &NodeEnv{
		cache: cache,
	}

CheckNodejs:
	output, err := exec.Command("npm", "version", "--json").CombinedOutput()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			PATH := os.Getenv("PATH")
			if !strings.Contains(PATH, "/usr/local/lib/nodejs/bin") {
				os.Setenv("PATH", fmt.Sprintf("/usr/local/lib/nodejs/bin%c%s", os.PathListSeparator, PATH))
				goto CheckNodejs
			}
			err = installNodejs(nodejsLatestLTS)
			if err != nil {
				return
			}
			log.Infof("nodejs %s installed", nodejsLatestLTS)
			goto CheckNodejs
		} else {
			err = errors.New("bad npm version")
		}
		return
	}

	json.NewDecoder(bytes.NewReader(output)).Decode(&env.version)
	if err != nil {
		err = errors.New("bad npm version")
		return
	}

	s, _ := utils.SplitByFirstByte(env.version.Node, '.')
	major, err := strconv.Atoi(s)
	if err != nil {
		err = errors.New("bad nodejs version")
		return
	}
	if major < minNodejsVersion {
		err = fmt.Errorf("bad nodejs version %s need %d+", env.version.Node, minNodejsVersion)
		return
	}

	output, err = exec.Command("npm", "config", "get", "registry").CombinedOutput()
	if err != nil {
		err = errors.New("bad registry config")
		return
	}
	env.registry = strings.TrimRight(strings.TrimSpace(string(output)), "/") + "/"

CheckYarn:
	output, err = exec.Command("yarn", "-v").CombinedOutput()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			output, err = exec.Command("npm", "install", "yarn", "-g").CombinedOutput()
			if err != nil {
				err = errors.New("install yarn: " + strings.TrimSpace(string(output)))
				return
			}
			goto CheckYarn
		}
		err = errors.New("bad yarn version")
		return
	}
	env.version.Yarn = strings.TrimSpace(string(output))
	return
}

func installNodejs(v string) (err error) {
	dlURL := fmt.Sprintf("https://npm.taobao.org/mirrors/node/v%s/node-v%s-%s-x64.tar.xz", v, v, runtime.GOOS)
	resp, err := http.Get(dlURL)
	if err != nil {
		err = fmt.Errorf("download nodejs: %v", err)
		return
	}
	defer resp.Body.Close()

	savePath := path.Join(os.TempDir(), path.Base(dlURL))
	f, err := os.Create(savePath)
	if err != nil {
		return
	}
	io.Copy(f, resp.Body)
	f.Close()

	cmd := exec.Command("tar", "-xJf", path.Base(dlURL))
	cmd.Dir = os.TempDir()
	output, err := cmd.CombinedOutput()
	if err != nil {
		if len(output) > 0 {
			err = errors.New(string(output))
		}
		return
	}

	cmd = exec.Command("mv", "-f", strings.TrimSuffix(path.Base(dlURL), ".tar.xz"), "/usr/local/lib/nodejs")
	cmd.Dir = os.TempDir()
	output, err = cmd.CombinedOutput()
	if err != nil {
		if len(output) > 0 {
			err = errors.New(string(output))
		}
	}
	return
}

func (env *NodeEnv) getPackageLatestInfo(name string) (info NpmPackage, err error) {
	value, err := env.cache.Get(name)
	if err == nil {
		info = NpmPackage{Name: name, Version: string(value)}
		return
	}
	if err != nil && err != cache.ErrExpired && err != cache.ErrNotFound {
		return
	}

	resp, err := http.Get(env.registry + name + "/latest")
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		err = fmt.Errorf("npm: package '%s' not found", name)
		return
	} else if resp.StatusCode != 200 {
		ret, _ := ioutil.ReadAll(resp.Body)
		log.Warnf("npm: can't get metadata of package '%s' (%d: %s)", name, resp.StatusCode, string(ret))
		err = fmt.Errorf("npm: can't get metadata of package '%s' (%d)", name, resp.StatusCode)
		return
	}

	err = json.NewDecoder(resp.Body).Decode(&info)
	if err == nil {
		env.cache.SetTTL(name, []byte(info.Version), 10*time.Second)
	}
	return
}

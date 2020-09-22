package server

import (
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
	dlURI            = "https://npm.taobao.org/mirrors/node/"
)

// NodeEnv defines the nodejs env
type NodeEnv struct {
	cache    cache.Cache
	version  string
	registry string
}

func checkNodeEnv() (env *NodeEnv, err error) {
	cache, err := cache.New("memory")
	if err != nil {
		return
	}
	env = &NodeEnv{
		cache:    cache,
		registry: "https://registry.npmjs.org/",
	}

	var installed bool
CheckNodejs:
	version, major, err := getSystemNodejsVersion()
	if err != nil || major < minNodejsVersion {
		PATH := os.Getenv("PATH")
		nodeBinDir := path.Join(etcDir, "/nodejs/bin")
		if !strings.Contains(PATH, nodeBinDir) {
			os.Setenv("PATH", fmt.Sprintf("%s%c%s", nodeBinDir, os.PathListSeparator, PATH))
			goto CheckNodejs
		} else if !installed {
			err = installNodejs(path.Join(etcDir, "/nodejs"), nodejsLatestLTS)
			if err != nil {
				return
			}
			log.Infof("nodejs %s installed", nodejsLatestLTS)
			installed = true
			goto CheckNodejs
		} else {
			if err == nil {
				err = fmt.Errorf("bad nodejs version %s need %d+", env.version, minNodejsVersion)
			}
			return
		}
	}
	env.version = version

	output, err := exec.Command("npm", "config", "get", "registry").CombinedOutput()
	if err == nil {
		env.registry = strings.TrimRight(strings.TrimSpace(string(output)), "/") + "/"
	}

CheckPnpm:
	output, err = exec.Command("pnpm", "-v").CombinedOutput()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			output, err = exec.Command("npm", "install", "pnpm", "-g").CombinedOutput()
			if err != nil {
				err = errors.New("install pnpm: " + strings.TrimSpace(string(output)))
				return
			}
			goto CheckPnpm
		}
		err = errors.New("bad pnpm version")
	}
	return
}

func getSystemNodejsVersion() (version string, major int, err error) {
	output, err := exec.Command("node", "--version").CombinedOutput()
	if err != nil {
		return
	}

	version = strings.TrimPrefix(strings.TrimSpace(string(output)), "v")
	s, _ := utils.SplitByFirstByte(version, '.')
	major, err = strconv.Atoi(s)
	return
}

func installNodejs(dir string, version string) (err error) {
	dlURL := fmt.Sprintf("%sv%s/node-v%s-%s-x64.tar.xz", dlURI, version, version, runtime.GOOS)
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

	cmd = exec.Command("mv", "-f", strings.TrimSuffix(path.Base(dlURL), ".tar.xz"), dir)
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
		err = fmt.Errorf("npm: can't get metadata of package '%s' (%s: %s)", name, resp.Status, string(ret))
		return
	}

	err = json.NewDecoder(resp.Body).Decode(&info)
	if err == nil {
		env.cache.SetTTL(name, []byte(info.Version), 10*time.Second)
	}
	return
}

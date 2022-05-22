package server

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ije/gox/utils"
	"github.com/ije/gox/valid"
)

var (
	regFullVersion      = regexp.MustCompile(`^\d+\.\d+\.\d+[a-zA-Z0-9\.\+\-_]*$`)
	regFullVersionPath  = regexp.MustCompile(`([^/])@\d+\.\d+\.\d+[a-zA-Z0-9\.\+\-_]*(/|$)`)
	regBuildVersionPath = regexp.MustCompile(`^/v\d+/`)
	regLocPath          = regexp.MustCompile(`(\.[a-z]+):\d+:\d+$`)
	npmNaming           = valid.Validator{valid.FromTo{'a', 'z'}, valid.FromTo{'0', '9'}, valid.Eq('.'), valid.Eq('_'), valid.Eq('-')}
)

type stringSet struct {
	lock sync.RWMutex
	m    map[string]struct{}
}

func newStringSet() *stringSet {
	return &stringSet{m: map[string]struct{}{}}
}

func (s *stringSet) Size() int {
	s.lock.RLock()
	defer s.lock.RUnlock()

	return len(s.m)
}

func (s *stringSet) Has(key string) bool {
	s.lock.RLock()
	defer s.lock.RUnlock()

	_, ok := s.m[key]
	return ok
}

func (s *stringSet) Add(key string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.m[key] = struct{}{}
}

func (s *stringSet) Values() []string {
	s.lock.RLock()
	defer s.lock.RUnlock()

	a := make([]string, len(s.m))
	i := 0
	for key := range s.m {
		a[i] = key
		i++
	}
	return a
}

func identify(importPath string) string {
	p := []byte(importPath)
	for i, c := range p {
		switch c {
		case '/', '-', '@', '.':
			p[i] = '_'
		default:
			p[i] = c
		}
	}
	return string(p)
}

func isRemoteImport(importPath string) bool {
	return strings.HasPrefix(importPath, "https://") || strings.HasPrefix(importPath, "http://")
}

func isLocalImport(importPath string) bool {
	return strings.HasPrefix(importPath, "file://") || strings.HasPrefix(importPath, "/") || strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../") || importPath == "." || importPath == ".."
}

func startsWith(s string, prefixs ...string) bool {
	for _, prefix := range prefixs {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

func endsWith(s string, suffixs ...string) bool {
	for _, suffix := range suffixs {
		if strings.HasSuffix(s, suffix) {
			return true
		}
	}
	return false
}

type devFS struct {
	cwd string
}

func (fs devFS) ReadFile(name string) ([]byte, error) {
	return ioutil.ReadFile(path.Join(fs.cwd, name))
}

func dirExists(filepath string) bool {
	fi, err := os.Lstat(filepath)
	return err == nil && fi.IsDir()
}

func fileExists(filepath string) bool {
	fi, err := os.Lstat(filepath)
	return err == nil && !fi.IsDir()
}

func ensureDir(dir string) (err error) {
	_, err = os.Stat(dir)
	if err != nil && os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0755)
	}
	return
}

func clearDir(dir string) (err error) {
	os.RemoveAll(dir)
	err = os.MkdirAll(dir, 0755)
	return
}

func btoaUrl(s string) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString([]byte(s)), "=")
}

func atobUrl(s string) (string, error) {
	if l := len(s) % 4; l > 0 {
		s += strings.Repeat("=", 4-l)
	}
	data, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func kill(pidFile string) (err error) {
	data, err := ioutil.ReadFile(pidFile)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	return process.Kill()
}

func cron(d time.Duration, task func()) {
	ticker := time.NewTicker(d)
	for {
		<-ticker.C
		task()
	}
}

func decodeAliasPrefix(raw string) (alias map[string]string, deps PkgSlice, err error) {
	s, err := atobUrl(strings.TrimPrefix(strings.TrimSuffix(raw, "/"), "X-"))
	if err == nil {
		for _, p := range strings.Split(s, "\n") {
			if strings.HasPrefix(p, "a/") || strings.HasPrefix(p, "alias:") {
				alias = map[string]string{}
				for _, p := range strings.Split(strings.TrimPrefix(strings.TrimPrefix(p, "a/"), "alias:"), ",") {
					p = strings.TrimSpace(p)
					if p != "" {
						name, to := utils.SplitByFirstByte(p, ':')
						name = strings.TrimSpace(name)
						to = strings.TrimSpace(to)
						if name != "" && to != "" {
							alias[name] = to
						}
					}
				}
			} else if strings.HasPrefix(p, "d/") || strings.HasPrefix(p, "deps:") {
				for _, p := range strings.Split(strings.TrimPrefix(strings.TrimPrefix(p, "d/"), "deps:"), ",") {
					p = strings.TrimSpace(p)
					if p != "" {
						m, _, err := parsePkg(p)
						if err != nil {
							if strings.HasSuffix(err.Error(), "not found") {
								continue
							}
							return nil, nil, err
						}
						if !deps.Has(m.Name) {
							deps = append(deps, *m)
						}
					}
				}
			}
		}
	}
	return
}

func encodeAliasPrefix(alias map[string]string, deps PkgSlice) string {
	args := []string{}
	if len(alias) > 0 {
		var ss sort.StringSlice
		for name, to := range alias {
			ss = append(ss, fmt.Sprintf("%s:%s", name, to))
		}
		ss.Sort()
		args = append(args, fmt.Sprintf("a/%s", strings.Join(ss, ",")))
	}
	if len(deps) > 0 {
		var ss sort.StringSlice
		for _, pkg := range deps {
			ss = append(ss, fmt.Sprintf("%s@%s", pkg.Name, pkg.Version))
		}
		ss.Sort()
		args = append(args, fmt.Sprintf("d/%s", strings.Join(ss, ",")))
	}
	if len(args) > 0 {
		return fmt.Sprintf("X-%s/", btoaUrl(strings.Join(args, "\n")))
	}
	return ""
}

func getOrigin(host string) string {
	proto := "https"
	if host == "localhost" || strings.HasPrefix(host, "localhost:") {
		proto = "http"
	}
	return fmt.Sprintf("%s://%s", proto, host)
}

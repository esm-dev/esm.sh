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
	regBuildVersionPath = regexp.MustCompile(`^/v\d+(/|$)`)
	regLocPath          = regexp.MustCompile(`(\.[a-z]+):\d+:\d+$`)
	npmNaming           = valid.Validator{valid.FromTo{'a', 'z'}, valid.FromTo{'A', 'Z'}, valid.FromTo{'0', '9'}, valid.Eq('.'), valid.Eq('_'), valid.Eq('-')}
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

func (s *stringSet) Remove(key string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	delete(s.m, key)
}

func (s *stringSet) Reset() {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.m = map[string]struct{}{}
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

func includes(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
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
	if pidFile == "" {
		return
	}
	data, err := ioutil.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			err = nil
		}
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

// func cron(d time.Duration, task func()) {
// 	ticker := time.NewTicker(d)
// 	for {
// 		<-ticker.C
// 		task()
// 	}
// }

func decodeBuildArgsPrefix(raw string) (args BuildArgs, err error) {
	s, err := atobUrl(strings.TrimPrefix(strings.TrimSuffix(raw, "/"), "X-"))
	if err == nil {
		args = BuildArgs{external: newStringSet()}
		for _, p := range strings.Split(s, "\n") {
			if strings.HasPrefix(p, "a/") {
				args.alias = map[string]string{}
				for _, p := range strings.Split(strings.TrimPrefix(strings.TrimPrefix(p, "a/"), "alias:"), ",") {
					name, to := utils.SplitByFirstByte(p, ':')
					name = strings.TrimSpace(name)
					to = strings.TrimSpace(to)
					if name != "" && to != "" {
						args.alias[name] = to
					}
				}
			} else if strings.HasPrefix(p, "d/") {
				for _, p := range strings.Split(strings.TrimPrefix(strings.TrimPrefix(p, "d/"), "deps:"), ",") {
					m, _, err := parsePkg(p)
					if err != nil {
						if strings.HasSuffix(err.Error(), "not found") {
							continue
						}
						return args, err
					}
					if !args.deps.Has(m.Name) {
						args.deps = append(args.deps, *m)
					}
				}
			} else if strings.HasPrefix(p, "e/") {
				for _, name := range strings.Split(strings.TrimPrefix(p, "e/"), ",") {
					args.external.Add(name)
				}
			} else if strings.HasPrefix(p, "dsv/") {
				args.denoStdVersion = strings.TrimPrefix(p, "dsv/")
			} else {
				switch p {
				case "ir":
					args.ignoreRequire = true
				case "kn":
					args.keepNames = true
				case "ia":
					args.ignoreAnnotations = true
				case "sm":
					args.sourcemap = true
				}
			}
		}
	}
	return
}

func encodeBuildArgsPrefix(args BuildArgs, pkg Pkg, forTypes bool) string {
	lines := []string{}
	pkgDeps := map[string]bool{}
	for i := 0; i < 3; i++ {
		info, _, err := getPackageInfo("", pkg.Name, pkg.Version)
		if err == nil {
			for name := range info.Dependencies {
				pkgDeps[name] = true
			}
			for name := range info.PeerDependencies {
				pkgDeps[name] = true
			}
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(args.alias) > 0 && !stableBuild[pkg.Name] {
		var ss sort.StringSlice
		for name, to := range args.alias {
			if name != pkg.Name && pkgDeps[name] {
				ss = append(ss, fmt.Sprintf("%s:%s", name, to))
			}
		}
		if len(ss) > 0 {
			ss.Sort()
			lines = append(lines, fmt.Sprintf("a/%s", strings.Join(ss, ",")))
		}
	}
	if len(args.deps) > 0 && !stableBuild[pkg.Name] {
		var ss sort.StringSlice
		for _, p := range args.deps {
			if p.Name != pkg.Name && pkgDeps[p.Name] {
				ss = append(ss, fmt.Sprintf("%s@%s", p.Name, p.Version))
			}
		}
		if len(ss) > 0 {
			ss.Sort()
			lines = append(lines, fmt.Sprintf("d/%s", strings.Join(ss, ",")))
		}
	}
	if args.external.Size() > 0 {
		var ss sort.StringSlice
		for _, name := range args.external.Values() {
			if name != pkg.Name {
				ss = append(ss, name)
			}
		}
		if len(ss) > 0 {
			ss.Sort()
			lines = append(lines, fmt.Sprintf("e/%s", strings.Join(ss, ",")))
		}
	}
	if !forTypes {
		if args.denoStdVersion != "" && args.denoStdVersion != denoStdVersion {
			lines = append(lines, fmt.Sprintf("dsv/%s", args.denoStdVersion))
		}
		if args.ignoreRequire {
			lines = append(lines, "ir")
		}
		if args.keepNames {
			lines = append(lines, "kn")
		}
		if args.ignoreAnnotations {
			lines = append(lines, "ia")
		}
		if args.sourcemap {
			lines = append(lines, "sm")
		}
	}
	if len(lines) > 0 {
		return fmt.Sprintf("X-%s/", btoaUrl(strings.Join(lines, "\n")))
	}
	return ""
}

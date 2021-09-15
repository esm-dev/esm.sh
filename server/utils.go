package server

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/ije/gox/utils"
	"github.com/ije/gox/valid"
)

var (
	regFullVersion      = regexp.MustCompile(`^\d+\.\d+\.\d+[a-zA-Z0-9\.\-]*$`)
	regVersionPath      = regexp.MustCompile(`([^/])@\d+\.\d+\.\d+([a-z0-9\.-]+)?/`)
	regBuildVersionPath = regexp.MustCompile(`^/v\d+/`)
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

// sortable version slice
type versionSlice []string

func (s versionSlice) Len() int      { return len(s) }
func (s versionSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s versionSlice) Less(i, j int) bool {
	avs, aStage := utils.SplitByFirstByte(s[i], '-')
	bvs, bStage := utils.SplitByFirstByte(s[j], '-')
	av := strings.Split(avs, ".")
	bv := strings.Split(bvs, ".")
	if len(av) != 3 || len(bv) != 3 {
		return avs > bvs
	}
	if av[0] == bv[0] {
		if av[1] == bv[1] {
			if av[2] == bv[2] {
				return aStage > bStage
			}
			a2, _ := strconv.Atoi(av[2])
			b2, _ := strconv.Atoi(bv[2])
			return a2 > b2
		}
		a1, _ := strconv.Atoi(av[1])
		b1, _ := strconv.Atoi(bv[1])
		return a1 > b1
	}
	a0, _ := strconv.Atoi(av[0])
	b0, _ := strconv.Atoi(bv[0])
	return a0 > b0
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

func resolveOrigin(r *http.Request) string {
	cdnDomain := config.cdnDomain
	if cdnDomain == "localhost" || strings.HasPrefix(cdnDomain, "localhost:") {
		return fmt.Sprintf("http://%s/", cdnDomain)
	} else if cdnDomain != "" {
		if strings.ContainsRune(cdnDomain, '*') {
			return fmt.Sprintf("https://%s/", strings.Replace(cdnDomain, "*", r.Host, 1))
		}
		return fmt.Sprintf("https://%s/", cdnDomain)
	}
	return "/"
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

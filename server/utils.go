package server

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

const (
	// EOL defines the char of end of line
	EOL = "\n"
)

var (
	regBuildVerPath = regexp.MustCompile(`^/v\d+/`)
	regFullVersion  = regexp.MustCompile(`^\d+\.\d+\.\d+(\-[a-zA-Z0-9\.]+)*$`)
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

func (s *stringSet) Add(key string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.m[key] = struct{}{}
}

type stringMap struct {
	lock sync.RWMutex
	m    map[string]string
}

func newStringMap() *stringMap {
	return &stringMap{m: map[string]string{}}
}

func (s *stringMap) Size() int {
	s.lock.RLock()
	defer s.lock.RUnlock()

	return len(s.m)
}

func (s *stringMap) Has(key string) bool {
	s.lock.RLock()
	defer s.lock.RUnlock()

	_, ok := s.m[key]
	return ok
}

func (s *stringMap) Keys() []string {
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

func (s *stringMap) Values() []string {
	s.lock.RLock()
	defer s.lock.RUnlock()

	a := make([]string, len(s.m))
	i := 0
	for _, value := range s.m {
		a[i] = value
		i++
	}
	return a
}

func (s *stringMap) Entries() [][2]string {
	s.lock.RLock()
	defer s.lock.RUnlock()

	a := make([][2]string, len(s.m))
	i := 0
	for key, value := range s.m {
		a[i] = [2]string{key, value}
		i++
	}
	return a
}

func (s *stringMap) Set(key string, value string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.m[key] = value
}

// sortable version slice
type versionSlice []string

func (s versionSlice) Len() int      { return len(s) }
func (s versionSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s versionSlice) Less(i, j int) bool {
	a := strings.Split(s[i], ".")
	b := strings.Split(s[j], ".")
	if len(a) != 3 || len(b) != 3 {
		return s[i] > s[j]
	}
	a0, _ := strconv.Atoi(a[0])
	b0, _ := strconv.Atoi(b[0])
	if a0 == b0 {
		a1, _ := strconv.Atoi(a[1])
		b1, _ := strconv.Atoi(b[1])
		if a1 == b1 {
			a2, _ := strconv.Atoi(a[2])
			b2, _ := strconv.Atoi(b[2])
			return a2 > b2
		}
		return a1 > b1
	}
	return a0 > b0
}

func isValidatedESImportPath(importPath string) bool {
	return strings.HasPrefix(importPath, "/") || strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../") || importPath == "." || importPath == ".."
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

func ensureExt(path string, ext string) string {
	if !strings.HasSuffix(path, ext) {
		return path + ext
	}
	return path
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

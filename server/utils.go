package server

import (
	"os"
	"regexp"
	"strings"
	"sync"
)

const (
	// EOL defines the char of end of line
	EOL = "\n"
)

var (
	regFullVersion = regexp.MustCompile(`^\d+\.\d+\.\d+(\-[a-zA-Z0-9\.]+)*$`)
	regProcess     = regexp.MustCompile(`[^a-zA-Z0-9_\.\$'"]process\.`)
	regBuffer      = regexp.MustCompile(`[^a-zA-Z0-9_\.\$'"]Buffer\.`)
	regGlobal      = regexp.MustCompile(`[^a-zA-Z0-9_\.\$'"]global(\.|\[)`)
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

func (s *stringSet) Set(key string) {
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

func ensureDir(dir string) (err error) {
	_, err = os.Stat(dir)
	if err != nil && os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0755)
	}
	return
}

func fileExists(filepath string) bool {
	fi, err := os.Lstat(filepath)
	return err == nil && !fi.IsDir()
}

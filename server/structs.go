package server

import (
	"encoding/json"
	"io/ioutil"
	"path"
	"strings"
	"sync"
)

type devFS struct {
	cwd string
}

func (fs devFS) ReadFile(name string) ([]byte, error) {
	return ioutil.ReadFile(path.Join(fs.cwd, name))
}

type stringSet struct {
	lock sync.RWMutex
	set  map[string]struct{}
}

func newStringSet(keys ...string) *stringSet {
	set := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		set[key] = struct{}{}
	}
	return &stringSet{set: set}
}

func (s *stringSet) Len() int {
	s.lock.RLock()
	defer s.lock.RUnlock()

	return len(s.set)
}

func (s *stringSet) Has(key string) bool {
	s.lock.RLock()
	defer s.lock.RUnlock()

	_, ok := s.set[key]
	return ok
}

func (s *stringSet) Add(key string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.set[key] = struct{}{}
}

func (s *stringSet) Remove(key string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	delete(s.set, key)
}

func (s *stringSet) Reset() {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.set = map[string]struct{}{}
}

func (s *stringSet) Values() []string {
	s.lock.RLock()
	defer s.lock.RUnlock()

	a := make([]string, len(s.set))
	i := 0
	for key := range s.set {
		a[i] = key
		i++
	}
	return a
}

type StringOrMap struct {
	Value string
	Map   map[string]interface{}
}

func (a *StringOrMap) UnmarshalJSON(b []byte) error {
	if err := json.Unmarshal(b, &a.Value); err != nil {
		return json.Unmarshal(b, &a.Map)
	}
	return nil
}

func (a *StringOrMap) MainValue() string {
	if a.Value != "" {
		return a.Value
	}
	if a.Map != nil {
		v, ok := a.Map["."]
		if ok {
			s, isStr := v.(string)
			if isStr {
				return s
			}
		}
	}
	return ""
}

type SortedPaths []string

func (a SortedPaths) Len() int {
	return len(a)
}

func (a SortedPaths) Less(i, j int) bool {
	iParts := strings.Split(a[i], "/")
	jParts := strings.Split(a[j], "/")
	for k := 0; k < len(iParts) && k < len(jParts); k++ {
		if iParts[k] != jParts[k] {
			return iParts[k] < jParts[k]
		}
	}
	return len(iParts) < len(jParts)
}

func (a SortedPaths) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

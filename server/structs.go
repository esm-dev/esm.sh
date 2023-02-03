package server

import (
	"encoding/json"
	"io/ioutil"
	"path"
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

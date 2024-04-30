package server

import (
	"bytes"
	"container/list"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
)

type DevFS struct {
	cwd string
}

func (fs DevFS) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(path.Join(fs.cwd, name))
}

func (fs DevFS) Lstat(name string) (os.FileInfo, error) {
	return os.Lstat(path.Join(fs.cwd, name))
}

type StringSet struct {
	lock sync.RWMutex
	set  map[string]struct{}
}

func newStringSet(keys ...string) *StringSet {
	set := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		set[key] = struct{}{}
	}
	return &StringSet{set: set}
}

func (s *StringSet) Len() int {
	s.lock.RLock()
	defer s.lock.RUnlock()

	return len(s.set)
}

func (s *StringSet) Has(key string) bool {
	s.lock.RLock()
	defer s.lock.RUnlock()

	_, ok := s.set[key]
	return ok
}

func (s *StringSet) Add(key string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.set[key] = struct{}{}
}

func (s *StringSet) Remove(key string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	delete(s.set, key)
}

func (s *StringSet) Reset() {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.set = map[string]struct{}{}
}

func (s *StringSet) Values() []string {
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

func (s *StringSet) SortedValues() []string {
	values := sort.StringSlice(s.Values())
	sort.Sort(values)
	return values
}

type StringOrMap struct {
	Str string
	Map map[string]interface{}
}

func (a *StringOrMap) UnmarshalJSON(b []byte) error {
	if err := json.Unmarshal(b, &a.Str); err != nil {
		return json.Unmarshal(b, &a.Map)
	}
	return nil
}

func (a *StringOrMap) MainValue() string {
	if a.Str != "" {
		return a.Str
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

// The orderedMap type, has similar operations as the default map type
// copied from https://gitlab.com/c0b/go-ordered-json
type OrderedMap struct {
	lock sync.RWMutex
	m    map[string]interface{}
	l    *list.List
	keys map[string]*list.Element // the double linked list for delete and lookup to be O(1)
}

// Create a new orderedMap
func newOrderedMap() *OrderedMap {
	return &OrderedMap{
		m:    make(map[string]interface{}),
		l:    list.New(),
		keys: make(map[string]*list.Element),
	}
}

// Set sets value for particular key, this will remember the order of keys inserted
// but if the key already exists, the order is not updated.
func (om *OrderedMap) Set(key string, value interface{}) {
	om.lock.Lock()
	defer om.lock.Unlock()
	if _, ok := om.m[key]; !ok {
		om.keys[key] = om.l.PushBack(key)
	}
	om.m[key] = value
}

// Entry returns the key and value by the given list element
func (om *OrderedMap) Entry(e *list.Element) (string, interface{}) {
	key := e.Value.(string)
	return key, om.m[key]
}

// UnmarshalJSON implements type json.Unmarshaler interface, so can be called in json.Unmarshal(data, om)
func (om *OrderedMap) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	// must open with a delim token '{'
	t, err := dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := t.(json.Delim); !ok || delim != '{' {
		return fmt.Errorf("expect JSON object open with '{'")
	}

	err = om.parseObject(dec)
	if err != nil {
		return err
	}

	t, err = dec.Token()
	if err != io.EOF {
		return fmt.Errorf("expect end of JSON object but got more token: %T: %v or err: %v", t, t, err)
	}

	return nil
}

func (om *OrderedMap) parseObject(dec *json.Decoder) (err error) {
	var t json.Token
	for dec.More() {
		t, err = dec.Token()
		if err != nil {
			return err
		}

		key, ok := t.(string)
		if !ok {
			return fmt.Errorf("expecting JSON key should be always a string: %T: %v", t, t)
		}

		t, err = dec.Token()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		var value interface{}
		value, err = handleDelim(t, dec)
		if err != nil {
			return err
		}

		// om.keys = append(om.keys, key)
		om.keys[key] = om.l.PushBack(key)
		om.m[key] = value
	}

	t, err = dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := t.(json.Delim); !ok || delim != '}' {
		return fmt.Errorf("expect JSON object close with '}'")
	}

	return nil
}

func parseArray(dec *json.Decoder) (arr []interface{}, err error) {
	var t json.Token
	arr = make([]interface{}, 0)
	for dec.More() {
		t, err = dec.Token()
		if err != nil {
			return
		}

		var value interface{}
		value, err = handleDelim(t, dec)
		if err != nil {
			return
		}
		arr = append(arr, value)
	}
	t, err = dec.Token()
	if err != nil {
		return
	}
	if delim, ok := t.(json.Delim); !ok || delim != ']' {
		err = fmt.Errorf("expect JSON array close with ']'")
		return
	}

	return
}

func handleDelim(t json.Token, dec *json.Decoder) (res interface{}, err error) {
	if delim, ok := t.(json.Delim); ok {
		switch delim {
		case '{':
			om2 := newOrderedMap()
			err = om2.parseObject(dec)
			if err != nil {
				return
			}
			return om2, nil
		case '[':
			var value []interface{}
			value, err = parseArray(dec)
			if err != nil {
				return
			}
			return value, nil
		default:
			return nil, fmt.Errorf("unexpected delimiter: %q", delim)
		}
	}
	return t, nil
}

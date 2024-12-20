package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Set struct {
	lock sync.RWMutex
	set  map[string]struct{}
}

func NewSet(keys ...string) *Set {
	set := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		set[key] = struct{}{}
	}
	return &Set{set: set}
}

func (s *Set) Len() int {
	s.lock.RLock()
	defer s.lock.RUnlock()

	return len(s.set)
}

func (s *Set) Has(key string) bool {
	s.lock.RLock()
	defer s.lock.RUnlock()

	_, ok := s.set[key]
	return ok
}

func (s *Set) Add(key string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.set[key] = struct{}{}
}

func (s *Set) Remove(key string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	delete(s.set, key)
}

func (s *Set) Reset() {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.set = map[string]struct{}{}
}

func (s *Set) Values() []string {
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

func (s *Set) SortedValues() []string {
	slice := sort.StringSlice(s.Values())
	slice.Sort()
	return slice
}

type JsonAny struct {
	Str string
	Map map[string]any
	Any any
}

func (a *JsonAny) MarshalJSON() ([]byte, error) {
	if a.Str != "" {
		return json.Marshal(a.Str)
	}
	return json.Marshal(a.Map)
}

func (a *JsonAny) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		a.Str = s
		return nil
	}
	var m map[string]any
	if json.Unmarshal(b, &m) == nil {
		a.Map = m
		return nil
	}
	return json.Unmarshal(b, &a.Any)
}

func (a *JsonAny) String() string {
	if a.Str != "" {
		return a.Str
	}
	if a.Map != nil {
		if v, ok := a.Map["."]; ok {
			if s, isStr := v.(string); isStr {
				return s
			}
		}
	}
	return ""
}

type SortablePaths []string

func (a SortablePaths) Len() int {
	return len(a)
}
func (a SortablePaths) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}
func (a SortablePaths) Less(i, j int) bool {
	iParts := strings.Split(a[i], "/")
	jParts := strings.Split(a[j], "/")
	for k := 0; k < len(iParts) && k < len(jParts); k++ {
		if iParts[k] != jParts[k] {
			return iParts[k] < jParts[k]
		}
	}
	return len(iParts) < len(jParts)
}

// based on https://gitlab.com/c0b/go-ordered-json
type JsonObject struct {
	keys   []string
	values map[string]interface{}
}

// Create a new orderedMap
func newJSONObject() *JsonObject {
	return &JsonObject{
		values: make(map[string]interface{}),
	}
}

func (om *JsonObject) Len() int {
	return len(om.keys)
}

func (om *JsonObject) Get(key string) (interface{}, bool) {
	v, ok := om.values[key]
	return v, ok
}

// Set sets value for particular key, this will remember the order of keys inserted
// but if the key already exists, the order is not updated.
func (om *JsonObject) Set(key string, value interface{}) {
	if _, ok := om.values[key]; !ok {
		om.keys = append(om.keys, key)
	}
	om.values[key] = value
}

// UnmarshalJSON implements type json.Unmarshaler interface, so can be called in json.Unmarshal(data, om)
func (om *JsonObject) UnmarshalJSON(data []byte) error {
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

	err = om.parse(dec)
	if err != nil {
		return err
	}

	t, err = dec.Token()
	if err != io.EOF {
		return fmt.Errorf("expect end of JSON object but got more token: %T: %v or err: %v", t, t, err)
	}

	return nil
}

func (om *JsonObject) parse(dec *json.Decoder) (err error) {
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

		om.keys = append(om.keys, key)
		om.values[key] = value
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
			om2 := newJSONObject()
			err = om2.parse(dec)
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

type HttpClient struct {
	*http.Client
	userAgent string
}

func NewFetchClient(timeout time.Duration, userAgent string) *HttpClient {
	return &HttpClient{
		Client:    &http.Client{Timeout: timeout},
		userAgent: userAgent,
	}
}

func (c *HttpClient) Fetch(url *url.URL) (resp *http.Response, err error) {
	req := &http.Request{
		Method:     "GET",
		URL:        url,
		Host:       url.Host,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header: http.Header{
			"User-Agent": []string{c.userAgent},
		},
	}
	return c.Do(req)
}

type KeyedMutex struct {
	mutexes sync.Map
}

type KeyedMutexItem struct {
	lock  sync.Mutex
	count atomic.Int32
}

func (m *KeyedMutex) Lock(key string) func() {
	value, _ := m.mutexes.LoadOrStore(key, &KeyedMutexItem{})
	mtx := value.(*KeyedMutexItem)
	mtx.count.Add(1)
	mtx.lock.Lock()

	return func() {
		mtx.lock.Unlock()
		mtx.count.Add(-1)
		if mtx.count.Load() == 0 {
			m.mutexes.Delete(key)
		}
	}
}

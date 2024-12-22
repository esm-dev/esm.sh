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

type JSONAny struct {
	Str string
	Map map[string]any
	Any any
}

func (a *JSONAny) MarshalJSON() ([]byte, error) {
	if a.Str != "" {
		return json.Marshal(a.Str)
	}
	return json.Marshal(a.Map)
}

func (a *JSONAny) UnmarshalJSON(b []byte) error {
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

func (a *JSONAny) String() string {
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
type JSONObject struct {
	keys   []string
	values map[string]any
}

func (obj *JSONObject) Len() int {
	return len(obj.keys)
}

func (obj *JSONObject) Get(key string) (any, bool) {
	v, ok := obj.values[key]
	return v, ok
}

// Set sets value for particular key, this will remember the order of keys inserted
// but if the key already exists, the order is not updated.
func (obj *JSONObject) Set(key string, value any) {
	if _, ok := obj.values[key]; !ok {
		obj.keys = append(obj.keys, key)
	}
	obj.values[key] = value
}

// UnmarshalJSON implements type json.Unmarshaler interface
func (obj *JSONObject) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	t, err := dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := t.(json.Delim); !ok || delim != '{' {
		return fmt.Errorf("expect JSON object open with '{'")
	}

	err = obj.parse(dec)
	if err != nil {
		return err
	}

	t, err = dec.Token()
	if err != io.EOF {
		return fmt.Errorf("expect end of JSON object but got more token: %T: %v or err: %v", t, t, err)
	}

	return nil
}

func (obj *JSONObject) parse(dec *json.Decoder) (err error) {
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

		var value any
		value, err = handleDelim(t, dec)
		if err != nil {
			return err
		}

		obj.keys = append(obj.keys, key)
		obj.values[key] = value
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

func parseArray(dec *json.Decoder) (arr []any, err error) {
	var t json.Token
	arr = make([]any, 0)
	for dec.More() {
		t, err = dec.Token()
		if err != nil {
			return
		}

		var value any
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

func handleDelim(t json.Token, dec *json.Decoder) (res any, err error) {
	if delim, ok := t.(json.Delim); ok {
		switch delim {
		case '{':
			om2 := &JSONObject{
				values: make(map[string]any),
			}
			err = om2.parse(dec)
			if err != nil {
				return
			}
			return om2, nil
		case '[':
			var value []any
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

var clientPool = sync.Pool{
	New: func() any {
		return &HttpClient{Client: &http.Client{}}
	},
}

type HttpClient struct {
	*http.Client
	userAgent string
}

func NewFetchClient(timeout time.Duration, userAgent string) (client *HttpClient, recycle func()) {
	client = clientPool.Get().(*HttpClient)
	client.Client.Timeout = timeout
	client.userAgent = userAgent
	return client, func() {
		clientPool.Put(client)
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

// Once is an object that will perform exactly one action.
// Different from sync.Once, this implementation allows the once function
// to return an error, that doesn't update the done flag.
type Once struct {
	done atomic.Uint32
	lock sync.Mutex
}

func (o *Once) Do(f func() error) error {
	if o.done.Load() == 0 {
		// Outlined slow-path to allow inlining of the fast-path.
		return o.doSlow(f)
	}
	return nil
}

func (o *Once) doSlow(f func() error) error {
	o.lock.Lock()
	defer o.lock.Unlock()
	if o.done.Load() == 0 {
		err := f()
		if err == nil {
			o.done.Store(1)
		}
		return err
	}
	return nil
}

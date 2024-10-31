package server

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

var (
	fetchLocks sync.Map
	fetchCache sync.Map
)

type CacheItem struct {
	data []byte
	exp  time.Time
}

type Fetcher struct {
	client *http.Client
	ua     string
}

func newFetcher(ua string, timeout time.Duration) *Fetcher {
	return &Fetcher{&http.Client{
		Timeout: timeout,
	}, ua}
}

func (f *Fetcher) Fetch(url *url.URL) (resp *http.Response, err error) {
	req := &http.Request{
		Method:     "GET",
		URL:        url,
		Host:       url.Host,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header: http.Header{
			"User-Agent": []string{f.ua},
		},
	}
	return f.client.Do(req)
}

func fetchSync(key string, cacheTtl time.Duration, fn func() (io.Reader, error)) (r io.Reader, err error) {
	v, _ := fetchLocks.LoadOrStore(key, &sync.Mutex{})
	lock := v.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	// check cache first
	if v, ok := fetchCache.Load(key); ok {
		item := v.(*CacheItem)
		if item.exp.After(time.Now()) {
			return bytes.NewReader(item.data), nil
		}
		fetchCache.Delete(key)
	}

	r, err = fn()
	if err != nil {
		return
	}

	data, err := io.ReadAll(r)
	if closer, ok := r.(io.Closer); ok {
		closer.Close()
	}
	if err != nil {
		return
	}

	fetchCache.Store(key, &CacheItem{
		data: data,
		exp:  time.Now().Add(cacheTtl),
	})
	r = bytes.NewReader(data)
	return
}

func init() {
	// fetch cache gc
	go func() {
		for {
			time.Sleep(time.Minute)
			now := time.Now()
			expKeys := []any{}
			fetchCache.Range(func(key, value any) bool {
				item := value.(*CacheItem)
				if item.exp.Before(now) {
					expKeys = append(expKeys, key)
				}
				return true
			})
			for _, key := range expKeys {
				fetchCache.Delete(key)
			}
		}
	}()
}

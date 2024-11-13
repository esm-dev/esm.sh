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

type FetchClient struct {
	*http.Client
	userAgent string
}

var defaultFetchClient = &FetchClient{
	Client:    &http.Client{Timeout: 30 * time.Second},
	userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
}

func (f *FetchClient) Fetch(url *url.URL) (resp *http.Response, err error) {
	req := &http.Request{
		Method:     "GET",
		URL:        url,
		Host:       url.Host,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header: http.Header{
			"User-Agent": []string{f.userAgent},
		},
	}
	return f.Do(req)
}

func fetchSync(key string, cacheTtl time.Duration, fetch func() (io.Reader, error)) (r io.Reader, err error) {
	v, _ := fetchLocks.LoadOrStore(key, &sync.Mutex{})
	lock := v.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	// check cache first
	if v, ok := fetchCache.Load(key); ok {
		item := v.(*CacheItem)
		if item.exp.Before(time.Now()) {
			fetchCache.Delete(key)
		}
		return bytes.NewReader(item.data), nil
	}

	r, err = fetch()
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

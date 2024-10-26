package server

import (
	"net/http"
	"net/url"
	"time"
)

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

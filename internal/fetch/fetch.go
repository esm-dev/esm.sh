package fetch

import (
	"errors"
	"net/http"
	"net/url"
	"sync"
	"time"
)

var clientPool = sync.Pool{
	New: func() any {
		return &FetchClient{Client: &http.Client{}}
	},
}

// FetchClient is a custom HTTP client.
type FetchClient struct {
	*http.Client
	userAgent string
}

// NewClient creates a new FetchClient.
func NewClient(userAgent string, timeout int, reserveRedirect bool) (client *FetchClient, recycle func()) {
	client = clientPool.Get().(*FetchClient)
	client.userAgent = userAgent
	client.Timeout = time.Duration(timeout) * time.Second
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if reserveRedirect && len(via) > 0 {
			return http.ErrUseLastResponse
		}
		if len(via) >= 3 {
			return errors.New("stopped after 3 redirects")
		}
		return nil
	}
	return client, func() { clientPool.Put(client) }
}

// Do sends an HTTP request and returns the response.
func (c *FetchClient) Fetch(url *url.URL, header http.Header) (resp *http.Response, err error) {
	if c.userAgent != "" {
		if header == nil {
			header = make(http.Header)
		}
		header.Set("User-Agent", c.userAgent)
	}
	req := &http.Request{
		Method:     "GET",
		URL:        url,
		Host:       url.Host,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     header,
	}
	return c.Do(req)
}

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
	userAgent    string
	allowedHosts map[string]struct{}
}

// NewClient creates a new FetchClient.
func NewClient(userAgent string, timeout int, reserveRedirect bool, allowedHosts map[string]struct{}) (client *FetchClient, recycle func()) {
	client = clientPool.Get().(*FetchClient)
	client.userAgent = userAgent
	client.Timeout = time.Duration(timeout) * time.Second
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if reserveRedirect && len(via) > 0 {
			return http.ErrUseLastResponse
		}
		// To avoid SSRF attacks, we check if the request URL's host is in the allowed hosts list.
		if allowedHosts != nil {
			if _, ok := allowedHosts[req.URL.Host]; !ok {
				return http.ErrUseLastResponse
			}
		}
		if len(via) >= 6 {
			return errors.New("too many redirects")
		}
		return nil
	}
	return client, func() { clientPool.Put(client) }
}

// Do sends an HTTP request and returns the response.
func (c *FetchClient) Fetch(url *url.URL, header http.Header) (resp *http.Response, err error) {
	if c.allowedHosts != nil {
		if _, ok := c.allowedHosts[url.Host]; !ok {
			return nil, errors.New("host not allowed: " + url.Host)
		}
	}
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

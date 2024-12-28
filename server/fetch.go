package server

import (
	"net/http"
	"net/url"
	"sync"
	"time"
)

var fetchClientPool = sync.Pool{
	New: func() any {
		return &FetchClient{Client: &http.Client{}}
	},
}

type FetchClient struct {
	*http.Client
	userAgent string
}

func NewFetchClient(timeout int, userAgent string) (client *FetchClient, recycle func()) {
	client = fetchClientPool.Get().(*FetchClient)
	client.Client.Timeout = time.Duration(timeout) * time.Second
	client.userAgent = userAgent
	return client, func() { fetchClientPool.Put(client) }
}

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

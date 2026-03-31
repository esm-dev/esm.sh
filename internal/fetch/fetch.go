package fetch

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"
)

// FetchClient is a custom HTTP client.
type FetchClient struct {
	*http.Client
	userAgent string
}

// NewClient creates a new FetchClient.
func NewClient(userAgent string, timeout int, reserveRedirect bool) (client *FetchClient) {
	client = &FetchClient{Client: &http.Client{}}
	client.userAgent = userAgent
	client.Timeout = time.Duration(timeout) * time.Second
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if reserveRedirect && len(via) > 0 {
			return http.ErrUseLastResponse
		}
		if len(via) >= 6 {
			return errors.New("too many redirects")
		}
		return nil
	}
	return client
}

// Fetch sends an HTTP GET request to the specified URL and returns the response.
func (c *FetchClient) Fetch(url *url.URL, header http.Header) (resp *http.Response, err error) {
	return c.FetchWithContext(context.Background(), url, header)
}

// FetchWithContext sends an HTTP GET request with cancellation support.
func (c *FetchClient) FetchWithContext(ctx context.Context, url *url.URL, header http.Header) (resp *http.Response, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if c.userAgent != "" {
		if header == nil {
			header = make(http.Header)
		}
		header.Set("User-Agent", c.userAgent)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Host = url.Host
	req.Proto = "HTTP/1.1"
	req.ProtoMajor = 1
	req.ProtoMinor = 1
	req.Header = header
	return c.Do(req)
}

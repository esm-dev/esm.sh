package storage

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

type s3TestTransport func(*http.Request) (*http.Response, error)

func (transport s3TestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return transport(req)
}

type s3TestBody struct {
	io.Reader
	closed chan struct{}
}

func (body *s3TestBody) Close() error {
	close(body.closed)
	return nil
}

func TestS3StorageGetClosesInvalidResponse(t *testing.T) {
	for _, headers := range []http.Header{
		{},
		{"Content-Length": {"invalid"}},
		{"Content-Length": {"4"}},
		{"Content-Length": {"4"}, "Last-Modified": {"invalid"}},
	} {
		t.Run(headers.Get("Content-Length")+"/"+headers.Get("Last-Modified"), func(t *testing.T) {
			body := &s3TestBody{Reader: strings.NewReader("data"), closed: make(chan struct{})}
			client := http.DefaultClient
			t.Cleanup(func() { http.DefaultClient = client })
			http.DefaultClient = &http.Client{Transport: s3TestTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: headers, Body: body}, nil
			})}
			s3 := &s3Storage{apiEndpoint: "https://storage.test"}
			if _, _, err := s3.Get("test.txt"); err == nil {
				t.Fatal("expected a metadata error")
			}
			select {
			case <-body.closed:
			default:
				t.Fatal("response body was not closed")
			}
		})
	}
}

func TestS3StorageGetClosesResponseWhenCacheIsFilled(t *testing.T) {
	cache, err := NewFSStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	body := &s3TestBody{Reader: strings.NewReader("data"), closed: make(chan struct{})}
	client := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = client })
	http.DefaultClient = &http.Client{Transport: s3TestTransport(func(*http.Request) (*http.Response, error) {
		if err := cache.Put("test.txt", strings.NewReader("data")); err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode: 200,
			Header: http.Header{
				"Content-Length": {"4"},
				"Last-Modified":  {time.Now().UTC().Format(http.TimeFormat)},
			},
			Body: body,
		}, nil
	})}
	s3 := &s3Storage{apiEndpoint: "https://storage.test", fsCache: cache.(*fsStorage)}
	content, _, err := s3.Get("test.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer content.Close()
	data, err := io.ReadAll(content)
	if err != nil || string(data) != "data" {
		t.Fatalf("content %q, error %v", data, err)
	}
	select {
	case <-body.closed:
	case <-time.After(time.Second):
		t.Fatal("response body was not closed")
	}
}

func TestS3StorageFSCacheKey(t *testing.T) {
	s3 := &s3Storage{}
	hash := "3fdadb34f247fde94adfb18268ac0caeed539ae7ad1380035d1366943b85ca7a"
	got := s3.fsCacheKey("meta/" + hash)
	want := "meta/3f/dadb34f247fde94adfb18268ac0caeed539ae7ad1380035d1366943b85ca7a"
	if got != want {
		t.Fatalf("invalid cache key %q, want %q", got, want)
	}
	if got = s3.fsCacheKey("v135/react@19.2.0/esnext/react.mjs"); got != "v135/react@19.2.0/esnext/react.mjs" {
		t.Fatalf("invalid cache key %q", got)
	}
}

func TestS3StorageListPagination(t *testing.T) {
	client := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = client })
	const token = "next+/=&?"
	var previous *s3TestBody
	requests := 0
	http.DefaultClient = &http.Client{Transport: s3TestTransport(func(req *http.Request) (*http.Response, error) {
		requests++
		if previous != nil {
			select {
			case <-previous.closed:
			default:
				t.Error("previous page body was not closed")
			}
		}
		if req.URL.Query().Get("prefix") != "test/" || req.URL.Query().Get("list-type") != "2" {
			t.Fatalf("unexpected list query: %s", req.URL.RawQuery)
		}
		var body string
		switch requests {
		case 1:
			body = `<ListBucketResult><Contents><Key>test/a</Key></Contents><IsTruncated>true</IsTruncated><NextContinuationToken>next+/=&amp;?</NextContinuationToken></ListBucketResult>`
		case 2:
			if req.URL.Query().Get("continuation-token") != token {
				t.Fatalf("unexpected continuation token: %s", req.URL.RawQuery)
			}
			body = `<ListBucketResult><Contents><Key>test/b</Key></Contents><IsTruncated>false</IsTruncated></ListBucketResult>`
		default:
			t.Fatal("unexpected extra list request")
		}
		previous = &s3TestBody{Reader: strings.NewReader(body), closed: make(chan struct{})}
		return &http.Response{StatusCode: 200, Body: previous}, nil
	})}
	s3 := &s3Storage{apiEndpoint: "https://storage.test"}
	keys, err := s3.List("test/")
	if err != nil || !slices.Equal(keys, []string{"test/a", "test/b"}) {
		t.Fatalf("list = %v, %v", keys, err)
	}
	select {
	case <-previous.closed:
	default:
		t.Fatal("last page body was not closed")
	}
}

func TestS3StorageListInvalidPagination(t *testing.T) {
	for _, token := range []string{"", "repeated"} {
		t.Run(token, func(t *testing.T) {
			client := http.DefaultClient
			t.Cleanup(func() { http.DefaultClient = client })
			requests := 0
			http.DefaultClient = &http.Client{Transport: s3TestTransport(func(*http.Request) (*http.Response, error) {
				requests++
				if requests > 2 {
					t.Fatal("pagination did not stop on an invalid token")
				}
				body := "<ListBucketResult><IsTruncated>true</IsTruncated><NextContinuationToken>" + token + "</NextContinuationToken></ListBucketResult>"
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			s3 := &s3Storage{apiEndpoint: "https://storage.test"}
			if _, err := s3.List("test/"); err == nil {
				t.Fatal("expected invalid pagination to fail")
			}
		})
	}
}

func TestS3StorageDeleteAllBatches(t *testing.T) {
	client := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = client })
	keys := make([]string, 2005)
	for i := range keys {
		keys[i] = fmt.Sprintf("test/%04d", i)
	}
	var batches []int
	var deleted []string
	http.DefaultClient = &http.Client{Transport: s3TestTransport(func(req *http.Request) (*http.Response, error) {
		var response strings.Builder
		if req.Method == http.MethodGet {
			start := 0
			if _, err := fmt.Sscanf(req.URL.Query().Get("continuation-token"), "%d", &start); err != nil && req.URL.Query().Has("continuation-token") {
				t.Fatal(err)
			}
			end := min(start+1000, len(keys))
			response.WriteString("<ListBucketResult>")
			for _, key := range keys[start:end] {
				fmt.Fprintf(&response, "<Contents><Key>%s</Key></Contents>", key)
			}
			if end < len(keys) {
				fmt.Fprintf(&response, "<IsTruncated>true</IsTruncated><NextContinuationToken>%d</NextContinuationToken>", end)
			}
			response.WriteString("</ListBucketResult>")
		} else {
			if req.Method != http.MethodPost || !req.URL.Query().Has("delete") {
				t.Fatalf("unexpected delete request: %s %s", req.Method, req.URL)
			}
			data, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatal(err)
			}
			checksum := md5.Sum(data)
			if req.Header.Get("Content-MD5") != base64.StdEncoding.EncodeToString(checksum[:]) {
				t.Error("missing or incorrect delete body checksum")
			}
			var body struct{ Object []struct{ Key string } }
			if err := xml.Unmarshal(data, &body); err != nil {
				t.Fatal(err)
			}
			batches = append(batches, len(body.Object))
			response.WriteString("<DeleteResult>")
			for _, object := range body.Object {
				deleted = append(deleted, object.Key)
				fmt.Fprintf(&response, "<Deleted><Key>%s</Key></Deleted>", object.Key)
			}
			response.WriteString("</DeleteResult>")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response.String()))}, nil
	})}
	s3 := &s3Storage{apiEndpoint: "https://storage.test"}
	got, err := s3.DeleteAll("test/")
	if err != nil || !slices.Equal(got, keys) || !slices.Equal(deleted, keys) || !slices.Equal(batches, []int{1000, 1000, 5}) {
		t.Fatalf("delete: %d results, %d requested keys, batches %v, error %v", len(got), len(deleted), batches, err)
	}
}

func TestS3StorageDeleteAllPartialError(t *testing.T) {
	client := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = client })
	http.DefaultClient = &http.Client{Transport: s3TestTransport(func(req *http.Request) (*http.Response, error) {
		body := `<ListBucketResult><Contents><Key>test/a</Key></Contents><Contents><Key>test/b</Key></Contents></ListBucketResult>`
		if req.Method == http.MethodPost {
			body = `<DeleteResult><Deleted><Key>test/a</Key></Deleted><Error><Key>test/b</Key><Code>AccessDenied</Code><Message>Access Denied</Message></Error></DeleteResult>`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	s3 := &s3Storage{apiEndpoint: "https://storage.test"}
	deleted, err := s3.DeleteAll("test/")
	if !slices.Equal(deleted, []string{"test/a"}) || err == nil || !strings.Contains(err.Error(), "test/b") || !strings.Contains(err.Error(), "AccessDenied") {
		t.Fatalf("partial delete = %v, %v", deleted, err)
	}
}

func TestS3Storage(t *testing.T) {
	endpint := os.Getenv("GO_TEST_S3_ENDPOINT")
	if endpint == "" {
		t.Skip("env GO_TEST_S3_ENDPOINT not set")
	}
	s3, err := NewS3Storage(&StorageOptions{
		Type:            "s3",
		Endpoint:        endpint,
		Region:          os.Getenv("GO_TEST_S3_REGION"),
		AccessKeyID:     os.Getenv("GO_TEST_S3_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("GO_TEST_S3_SECRET_ACCESS_KEY"),
	})
	if err != nil {
		t.Fatal(err)
	}

	dirname := os.Getenv("GO_TEST_S3_ROOTDIR")
	if dirname == "" {
		dirname = "test"
	}

	// clean up
	_, err = s3.DeleteAll(dirname + "/")
	if err != nil {
		t.Fatal(err)
	}

	err = s3.Put(dirname+"/hello.txt", bytes.NewReader([]byte("Hello, world!")))
	if err != nil {
		t.Fatal(err)
	}
	err = s3.Put(dirname+"/foo/bar.txt", bytes.NewBufferString("foobar~"))
	if err != nil {
		t.Fatal(err)
	}
	err = s3.Put(dirname+"/%23/hello+world!", TeeReader(bytes.NewReader([]byte("Hello, world!")), io.Discard))
	if err != nil {
		t.Fatal(err)
	}

	keys, err := s3.List(dirname + "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 3 {
		t.Fatalf("invalid keys length(%d), expected 3", len(keys))
	}

	stat, err := s3.Stat(dirname + "/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	if stat.Size() != 13 {
		t.Fatalf("invalid size(%d), expected 13", stat.Size())
	}

	r, stat, err := s3.Get(dirname + "/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	if stat.Size() != 13 {
		t.Fatalf("invalid size(%d), expected 13", stat.Size())
	}

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "Hello, world!" {
		t.Fatalf("invalid content(%s), expected 'Hello, world!'", string(data))
	}

	stat, err = s3.Stat(dirname + "/foo/bar.txt")
	if err != nil {
		t.Fatal(err)
	}
	if stat.Size() != 7 {
		t.Fatalf("invalid size(%d), expected 7", stat.Size())
	}

	r, stat, err = s3.Get(dirname + "/foo/bar.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	if stat.Size() != 7 {
		t.Fatalf("invalid size(%d), expected 7", stat.Size())
	}

	data, err = io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "foobar~" {
		t.Fatalf("invalid content(%s), expected 'foobar~'", string(data))
	}

	err = s3.Delete(dirname + "/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	err = s3.Delete(dirname + "/%23/hello+world!")
	if err != nil {
		t.Fatal(err)
	}

	keys, err = s3.List(dirname + "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 {
		t.Fatalf("invalid keys length(%d), expected 1", len(keys))
	}

	deleted, err := s3.DeleteAll(dirname + "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 {
		t.Fatalf("invalid deleted keys length(%d), expected 1", len(deleted))
	}

	keys, err = s3.List(dirname + "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 0 {
		t.Fatalf("invalid keys length(%d), expected 0", len(keys))
	}
}

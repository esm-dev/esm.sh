package storage

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"testing/synctest"
	"time"

	bolt "go.etcd.io/bbolt"
)

func TestS3StorageTimeout(t *testing.T) {
	for _, slowBody := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			srvConn, cliConn := net.Pipe()
			defer srvConn.Close()
			defer cliConn.Close()
			transport := s3Transport.Clone()
			transport.DialContext = func(context.Context, string, string) (net.Conn, error) { return cliConn, nil }
			defer transport.CloseIdleConnections()
			client := http.DefaultClient
			defer func() { http.DefaultClient = client }()
			http.DefaultClient = &http.Client{Transport: transport}
			go func() {
				if _, err := http.ReadRequest(bufio.NewReader(srvConn)); err != nil || !slowBody {
					return
				}
				io.WriteString(srvConn, "HTTP/1.1 200 OK\r\nContent-Length: 4\r\nLast-Modified: "+time.Now().UTC().Format(http.TimeFormat)+"\r\n\r\nda")
				time.Sleep(2 * time.Minute)
				io.WriteString(srvConn, "ta")
			}()
			s3 := &s3Storage{apiEndpoint: "http://storage.test"}
			content, _, err := s3.Get("meta/viem@2.56.6/test")
			if !slowBody {
				var netErr net.Error
				if !errors.As(err, &netErr) || !netErr.Timeout() {
					t.Fatalf("expected a response header timeout, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(content)
			content.Close()
			if err != nil || string(data) != "data" {
				t.Fatalf("slow body was cut: content %q, error %v", data, err)
			}
		})
	}
}

func TestS3StorageBuildCancellation(t *testing.T) {
	for _, method := range []string{"HEAD", "GET", "PUT"} {
		t.Run(method, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s := &s3Storage{apiEndpoint: "https://storage.test"}
				client := http.DefaultClient
				defer func() { http.DefaultClient = client }()
				started := make(chan struct{})
				http.DefaultClient = &http.Client{Transport: s3TestTransport(func(req *http.Request) (*http.Response, error) {
					if req.Method != method {
						t.Errorf("method = %s, want %s", req.Method, method)
					}
					close(started)
					<-req.Context().Done()
					return nil, req.Context().Err()
				})}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				done := make(chan error, 1)
				go func() {
					var err error
					if method == "HEAD" {
						_, err = s.StatContext(ctx, "types/pkg@1/index.d.ts")
					} else if method == "GET" {
						_, _, err = s.GetContext(ctx, "npm-resolution-slots/v1/01.json")
					} else {
						err = s.PutContext(ctx, "types/pkg@1/index.d.ts", bytes.NewBufferString("export {}"))
					}
					done <- err
				}()
				<-started
				cancel()
				if err := <-done; !errors.Is(err, context.Canceled) {
					t.Fatalf("expected cancellation, got %v", err)
				}
			})
		})
	}
}

func TestS3StoragePutCache(t *testing.T) {
	for _, result := range []string{"success", "rejected", "transport error", "canceled"} {
		for _, original := range []string{"", "original"} {
			t.Run(result+"/"+original, func(t *testing.T) {
				cache, err := NewFSStorage(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				s3 := &s3Storage{apiEndpoint: "https://storage.test", fsCache: cache.(*fsStorage)}
				const key = "test.txt"
				if original != "" {
					if err := cache.Put(key, strings.NewReader(original)); err != nil {
						t.Fatal(err)
					}
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				client := http.DefaultClient
				t.Cleanup(func() { http.DefaultClient = client })
				transportErr := errors.New("upload failed")
				http.DefaultClient = &http.Client{Transport: s3TestTransport(func(req *http.Request) (*http.Response, error) {
					if req.Method != http.MethodPut {
						return &http.Response{StatusCode: 404, Body: http.NoBody}, nil
					}
					defer req.Body.Close()
					data, err := io.ReadAll(req.Body)
					if err != nil || string(data) != "replacement" {
						t.Fatalf("upload: content %q, error %v", data, err)
					}
					content, _, err := cache.Get(key)
					if original == "" {
						if content != nil {
							content.Close()
						}
						if !errors.Is(err, ErrNotFound) {
							t.Errorf("cache before response: got %v, want ErrNotFound", err)
						}
					} else {
						if err != nil {
							t.Fatal(err)
						}
						data, err := io.ReadAll(content)
						content.Close()
						if err != nil || string(data) != original {
							t.Errorf("cache before response: content %q, error %v", data, err)
						}
					}
					switch result {
					case "rejected":
						return &http.Response{StatusCode: 500, Body: http.NoBody}, nil
					case "transport error":
						return nil, transportErr
					case "canceled":
						cancel()
						return nil, req.Context().Err()
					default:
						return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
					}
				})}
				err = s3.PutContext(ctx, key, bytes.NewBufferString("replacement"))
				if (err == nil) != (result == "success") {
					t.Fatalf("Put returned %v", err)
				}
				if result == "transport error" && !errors.Is(err, transportErr) {
					t.Fatalf("Put returned %v, want %v", err, transportErr)
				}
				if result == "canceled" && !errors.Is(err, context.Canceled) {
					t.Fatalf("Put returned %v, want context.Canceled", err)
				}
				want := original
				if result == "success" {
					want = "replacement"
				}
				stat, statErr := s3.Stat(key)
				content, _, err := s3.Get(key)
				if content != nil {
					defer content.Close()
				}
				if want == "" {
					if !errors.Is(statErr, ErrNotFound) || !errors.Is(err, ErrNotFound) {
						t.Fatalf("failed upload remains readable: Stat error %v, Get error %v", statErr, err)
					}
				} else {
					if statErr != nil || err != nil {
						t.Fatalf("Stat error %v, Get error %v", statErr, err)
					}
					data, err := io.ReadAll(content)
					if err != nil || string(data) != want || stat.Size() != int64(len(want)) {
						t.Fatalf("cache after upload: content %q, size %d, error %v", data, stat.Size(), err)
					}
				}
			})
		}
	}
}

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

func TestS3StorageCacheRejectSymlinkEscape(t *testing.T) {
	for _, operation := range []string{"Get", "Put"} {
		t.Run(operation, func(t *testing.T) {
			root := t.TempDir()
			outside := t.TempDir()
			victim := filepath.Join(outside, "file.txt")
			if err := os.WriteFile(victim, []byte("original"), 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
				t.Fatal(err)
			}
			cache, err := NewFSStorage(root)
			if err != nil {
				t.Fatal(err)
			}
			client := http.DefaultClient
			t.Cleanup(func() { http.DefaultClient = client })
			http.DefaultClient = &http.Client{Transport: s3TestTransport(func(req *http.Request) (*http.Response, error) {
				if req.Body != nil {
					defer req.Body.Close()
					if _, err := io.Copy(io.Discard, req.Body); err != nil {
						return nil, err
					}
				}
				return &http.Response{
					StatusCode: 200,
					Header: http.Header{
						"Content-Length": {"11"},
						"Last-Modified":  {time.Now().UTC().Format(http.TimeFormat)},
					},
					Body: io.NopCloser(strings.NewReader("replacement")),
				}, nil
			})}
			s3 := &s3Storage{apiEndpoint: "https://storage.test", fsCache: cache.(*fsStorage)}
			if operation == "Put" {
				err = s3.Put("link/file.txt", strings.NewReader("replacement"))
			} else {
				var content io.ReadCloser
				content, _, err = s3.Get("link/file.txt")
				if err == nil {
					var data []byte
					data, err = io.ReadAll(content)
					content.Close()
					if err == nil && string(data) != "replacement" {
						t.Errorf("Get: content %q", data)
					}
				}
			}
			if err != nil {
				t.Errorf("%s failed on a rejected cache key: %v", operation, err)
			}
			if data, err := os.ReadFile(victim); err != nil || string(data) != "original" {
				t.Fatalf("outside file changed: content %q, error %v", data, err)
			}
		})
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

func TestS3StoragePutEmpty(t *testing.T) {
	seen := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- fmt.Sprint(r.TransferEncoding, r.ContentLength)
	}))
	defer srv.Close()
	cache, err := NewFSStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s3 := &s3Storage{apiEndpoint: srv.URL, fsCache: cache.(*fsStorage)}
	if err := s3.Put("modules/empty.mjs", strings.NewReader("")); err != nil {
		t.Fatal(err)
	}
	if got := <-seen; got != "[] 0" {
		t.Fatalf("empty upload: transfer encoding and length %s, want [] 0", got)
	}
	if stat, err := cache.Stat("modules/empty.mjs"); err != nil || stat.Size() != 0 {
		t.Fatalf("empty cache file: %v", err)
	}
}

func TestS3StorageMetaCache(t *testing.T) {
	var requests []string
	client := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = client })
	http.DefaultClient = &http.Client{Transport: s3TestTransport(func(req *http.Request) (*http.Response, error) {
		requests = append(requests, req.Method+" "+req.URL.Path)
		switch req.Method {
		case http.MethodGet:
			if req.URL.RawQuery == "list-type=2&prefix=meta%2Fpkg%401.0.0%2F" {
				body := "<ListBucketResult><Contents><Key>meta/pkg@1.0.0/a</Key></Contents></ListBucketResult>"
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			}
			return &http.Response{StatusCode: 200, Header: http.Header{
				"Content-Length": {"6"},
				"Last-Modified":  {"Tue, 20 Oct 2026 00:00:00 GMT"},
			}, Body: io.NopCloser(strings.NewReader("remote"))}, nil
		case http.MethodPost:
			body := "<DeleteResult><Deleted><Key>meta/pkg@1.0.0/a</Key></Deleted></DeleteResult>"
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		}
		if req.Body != nil {
			io.Copy(io.Discard, req.Body)
		}
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	})}
	cacheDir := t.TempDir()
	options := &StorageOptions{Endpoint: "https://storage.test", AccessKeyID: "test", SecretAccessKey: "test", CacheDir: cacheDir, MetaCacheFile: filepath.Join(t.TempDir(), "meta-cache.db")}
	storage, err := NewS3Storage(options)
	if err != nil {
		t.Fatal(err)
	}
	s3 := storage.(*s3Storage)
	t.Cleanup(func() { s3.metaCache.Close() })
	read := func(name string) string {
		content, _, err := s3.Get(name)
		if err != nil {
			t.Fatal(err)
		}
		defer content.Close()
		data, err := io.ReadAll(content)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	if err := s3.metaCache.Update(func(tx *bolt.Tx) error {
		for _, name := range []string{"meta/pkg@1.0.0/a", "meta/pkg@1.0.0/b", "meta/pkg@1.0.0/c", "meta/pkg@1.0.10/a", "meta-purged/pkg@1.0.0"} {
			value := binary.BigEndian.AppendUint64(nil, uint64(time.Now().UnixMilli()))
			if strings.HasPrefix(name, "meta/") {
				value = append(value, name...)
			}
			if err := tx.Bucket(metaCacheBucket).Put([]byte(name), value); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	requests = nil
	if got := read("meta/pkg@1.0.0/b"); got != "meta/pkg@1.0.0/b" {
		t.Fatalf("cached content %q", got)
	}
	if stat, err := s3.Stat("meta-purged/pkg@1.0.0"); err != nil || stat.Size() != 0 {
		t.Fatalf("cached marker: %v", err)
	}
	if len(requests) != 0 {
		t.Fatalf("cached reads hit S3: %v", requests)
	}
	if got := read("meta/remote"); got != "remote" {
		t.Fatalf("remote content %q", got)
	}
	if _, stat := s3.getMetaCache("meta/remote"); stat != nil {
		t.Fatal("a metadata import filled the old cache")
	}
	if got := read("meta/remote"); got != "remote" || len(requests) != 2 {
		t.Fatalf("metadata import changed the old cache: %q, %v", got, requests)
	}
	if entries, err := os.ReadDir(cacheDir); err != nil || len(entries) != 0 {
		t.Fatalf("meta keys were cached as files: %v, %v", entries, err)
	}
	if err := s3.fsCache.Put("meta/old-file", strings.NewReader("old")); err != nil {
		t.Fatal(err)
	}
	requests = nil
	if got := read("meta/old-file"); got != "old" || len(requests) != 0 {
		t.Fatalf("old cache file was not reused: %q, %v", got, requests)
	}

	if err := s3.Delete("meta/remote"); err != nil {
		t.Fatal(err)
	}
	if _, err := s3.DeleteAll("meta/pkg@1.0.0/"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"meta/remote", "meta/pkg@1.0.0/a", "meta/pkg@1.0.0/b", "meta/pkg@1.0.0/c"} {
		if _, stat := s3.getMetaCache(name); stat != nil {
			t.Fatalf("deleted %s remains cached", name)
		}
	}
	for _, name := range []string{"meta/pkg@1.0.10/a", "meta-purged/pkg@1.0.0"} {
		if _, stat := s3.getMetaCache(name); stat == nil {
			t.Fatalf("%s was deleted by an unrelated prefix", name)
		}
	}
}

func TestS3StorageLocalCache(t *testing.T) {
	var mu sync.Mutex
	var requests []string
	remote := map[string]string{}
	client := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = client })
	http.DefaultClient = &http.Client{Transport: s3TestTransport(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		requests = append(requests, req.Method+" "+req.URL.Path)
		if req.Method == http.MethodPut {
			data, _ := io.ReadAll(req.Body)
			remote[req.URL.Path] = string(data)
			return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
		}
		data, ok := remote[req.URL.Path]
		if !ok {
			return &http.Response{StatusCode: 404, Body: http.NoBody}, nil
		}
		return &http.Response{StatusCode: 200, Header: http.Header{
			"Content-Length": {fmt.Sprint(len(data))},
			"Last-Modified":  {"Tue, 20 Oct 2026 00:00:00 GMT"},
		}, Body: io.NopCloser(strings.NewReader(data))}, nil
	})}
	options := &StorageOptions{Endpoint: "https://storage.test", AccessKeyID: "test", SecretAccessKey: "test", CacheDir: t.TempDir(), MetaCacheFile: filepath.Join(t.TempDir(), "meta-cache.db")}
	storage, err := NewS3Storage(options)
	if err != nil {
		t.Fatal(err)
	}
	s3 := storage.(*s3Storage)
	t.Cleanup(func() { s3.metaCache.Close() })
	sent := func() []string {
		mu.Lock()
		defer mu.Unlock()
		defer func() { requests = nil }()
		return requests
	}
	read := func(name string) string {
		content, _, err := s3.Get(name)
		if err != nil {
			t.Fatalf("get %s: %v", name, err)
		}
		defer content.Close()
		data, _ := io.ReadAll(content)
		return string(data)
	}

	// a miss is asked once, a put ends it
	for _, name := range []string{"modules/pkg@1.0.0/es2022/pkg.mjs", "meta/pkg@1.0.0/a"} {
		for range 2 {
			if _, _, err := s3.Get(name); err != ErrNotFound {
				t.Fatalf("get %s: %v", name, err)
			}
			if _, err := s3.Stat(name); err != ErrNotFound {
				t.Fatalf("stat %s: %v", name, err)
			}
		}
		if got := sent(); len(got) != 1 {
			t.Fatalf("a miss of %s was asked again: %v", name, got)
		}
		if err := s3.Put(name, strings.NewReader("data")); err != nil {
			t.Fatal(err)
		}
		if got := read(name); got != "data" {
			t.Fatalf("content of %s after a miss: %q", name, got)
		}
		want := []string{"PUT /" + name}
		if isMetaKey(name) {
			want = append(want, "GET /"+name)
		}
		if got := sent(); !slices.Equal(got, want) {
			t.Fatalf("requests after the put of %s: %v", name, got)
		}
	}
	// a miss that raced with a put is dropped
	seq := s3.putSeq("modules/raced.mjs").Load()
	s3.markPut("modules/raced.mjs")
	s3.addMiss("modules/raced.mjs", seq)
	if s3.isMissing("modules/raced.mjs") {
		t.Fatal("a miss that raced with a put was recorded")
	}
	sent()

	// types and exports are never uploaded, the old ones are still read from S3
	remote["/types/pkg@1.0.0/old.d.ts"] = "old"
	for _, name := range []string{"types/pkg@1.0.0/index.d.ts", "exports/pkg@1.0.0/es2022/pkg_hash.mjs"} {
		if err := s3.Put(name, strings.NewReader("local")); err != nil {
			t.Fatal(err)
		}
		if got := read(name); got != "local" {
			t.Fatalf("content of %s: %q", name, got)
		}
	}
	if got := sent(); len(got) != 0 {
		t.Fatalf("local keys hit S3: %v", got)
	}
	if got := read("types/pkg@1.0.0/old.d.ts"); got != "old" {
		t.Fatalf("old types: %q", got)
	}
	if got := read("types/pkg@1.0.0/old.d.ts"); got != "old" || len(sent()) != 1 {
		t.Fatalf("old types were not cached: %q", got)
	}

	// a source map is uploaded on the first read
	const sourceMap = "modules/pkg@1.0.0/es2022/pkg.mjs.map"
	if err := s3.Put(sourceMap, strings.NewReader("map")); err != nil {
		t.Fatal(err)
	}
	if got := sent(); len(got) != 0 {
		t.Fatalf("source map was uploaded on put: %v", got)
	}
	if got := read(sourceMap); got != "map" {
		t.Fatalf("source map: %q", got)
	}
	for range 1000 {
		if _, err := s3.fsCache.Stat(sourceMap); err == ErrNotFound {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if got := sent(); !slices.Equal(got, []string{"PUT /" + sourceMap}) {
		t.Fatalf("source map upload: %v", got)
	}
	if got := read(sourceMap); got != "map" || !slices.Equal(sent(), []string{"GET /" + sourceMap}) {
		t.Fatalf("uploaded source map: %q", got)
	}
	if _, err := s3.fsCache.Stat(sourceMap); err != ErrNotFound {
		t.Fatalf("uploaded source map was cached again: %v", err)
	}

	// a source map without a read is removed after the TTL
	const unused = "modules/pkg@1.0.0/es2022/unused.mjs.map"
	if err := s3.Put(unused, strings.NewReader("map")); err != nil {
		t.Fatal(err)
	}
	s3.sweepSourceMaps(time.Now().Add(-time.Hour))
	if _, err := s3.fsCache.Stat(unused); err != nil {
		t.Fatalf("fresh source map was removed: %v", err)
	}
	s3.sweepSourceMaps(time.Now().Add(time.Hour))
	if _, _, err := s3.Get(unused); err != ErrNotFound {
		t.Fatalf("expired source map: %v", err)
	}
	s3.metaCache.View(func(tx *bolt.Tx) error {
		if n := tx.Bucket(sourceMapsBucket).Stats().KeyN; n != 0 {
			t.Fatalf("%d source map records left", n)
		}
		return nil
	})
}

func TestS3StorageSignsWirePath(t *testing.T) {
	client := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = client })
	var paths []string
	http.DefaultClient = &http.Client{Transport: s3TestTransport(func(req *http.Request) (*http.Response, error) {
		uri, query, _ := strings.Cut(req.URL.RequestURI(), "?")
		paths = append(paths, req.Method+" "+uri)
		values, _ := url.ParseQuery(query)
		_, signed, _ := strings.Cut(req.Header.Get("Authorization"), "SignedHeaders=")
		signedHeaders, signature, _ := strings.Cut(signed, ", Signature=")
		var headers strings.Builder
		for key := range strings.SplitSeq(signedHeaders, ";") {
			value := strings.Join(req.Header.Values(key), ",")
			if key == "host" {
				value = req.Host
			}
			headers.WriteString(key + ":" + value + "\n")
		}
		datetime := req.Header.Get("X-Amz-Date")
		canonical := strings.Join([]string{req.Method, uri, values.Encode(), headers.String(), signedHeaders, "UNSIGNED-PAYLOAD"}, "\n")
		key := hmacSum(hmacSum(hmacSum(hmacSum([]byte("AWS4secret"), datetime[:8]), "us-east-1"), "s3"), "aws4_request")
		if toHex(hmacSum(key, "AWS4-HMAC-SHA256\n"+datetime+"\n"+datetime[:8]+"/us-east-1/s3/aws4_request\n"+toHex(sha256Sum(canonical)))) != signature {
			t.Errorf("%s %s: signature does not match the request line", req.Method, req.URL.RequestURI())
		}
		if req.Body != nil {
			io.Copy(io.Discard, req.Body)
		}
		body := "data"
		if values.Has("list-type") {
			body = "<ListBucketResult><Contents><Key>test/a</Key></Contents></ListBucketResult>"
		} else if values.Has("delete") {
			body = "<DeleteResult><Deleted><Key>test/a</Key></Deleted></DeleteResult>"
		}
		return &http.Response{StatusCode: 200, Header: http.Header{
			"Content-Length": {"4"},
			"Last-Modified":  {time.Now().UTC().Format(http.TimeFormat)},
		}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	s3 := &s3Storage{apiEndpoint: "https://storage.test", region: "us-east-1", accessKeyID: "id", secretAccessKey: "secret"}
	var want []string
	for key, path := range map[string]string{
		"modules/a%zz.mjs":      "/modules/a%25zz.mjs",
		"modules/\xff.mjs":      "/modules/%FF.mjs",
		"modules/a?b#c.mjs":     "/modules/a%3Fb%23c.mjs",
		"test/%23/hello+world!": "/test/%2523/hello%20world%21",
		"react@19.2.0/x.mjs":    "/react%4019.2.0/x.mjs",
	} {
		if _, err := s3.Stat(key); err != nil {
			t.Fatalf("Stat(%q): %v", key, err)
		}
		content, _, err := s3.Get(key)
		if err != nil {
			t.Fatalf("Get(%q): %v", key, err)
		}
		content.Close()
		if err := s3.Put(key, strings.NewReader("data")); err != nil {
			t.Fatalf("Put(%q): %v", key, err)
		}
		if err := s3.Delete(key); err != nil {
			t.Fatalf("Delete(%q): %v", key, err)
		}
		want = append(want, "HEAD "+path, "GET "+path, "PUT "+path, "DELETE "+path)
	}
	if _, err := s3.DeleteAll("test/"); err != nil {
		t.Fatal(err)
	}
	want = append(want, "GET /", "POST /")
	if !slices.Equal(paths, want) {
		t.Fatalf("request paths %q, want %q", paths, want)
	}
}

func TestS3StorageCacheFailure(t *testing.T) {
	cache, err := NewFSStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Put("file", strings.NewReader("")); err != nil {
		t.Fatal(err)
	}
	readErr := errors.New("connection reset")
	bodies := map[string]*s3TestBody{}
	var uploaded string
	client := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = client })
	http.DefaultClient = &http.Client{Transport: s3TestTransport(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodPut {
			data, err := io.ReadAll(req.Body)
			uploaded = string(data)
			return &http.Response{StatusCode: 200, Body: http.NoBody}, err
		}
		var reader io.Reader = strings.NewReader("data")
		if req.URL.Path == "/broken.mjs" {
			reader = io.MultiReader(strings.NewReader("da"), iotest.ErrReader(readErr))
		}
		body := &s3TestBody{Reader: reader, closed: make(chan struct{})}
		bodies[req.URL.Path] = body
		return &http.Response{StatusCode: 200, Header: http.Header{
			"Content-Length": {"4"},
			"Last-Modified":  {time.Now().UTC().Format(http.TimeFormat)},
		}, Body: body}, nil
	})}
	s3 := &s3Storage{apiEndpoint: "https://storage.test", fsCache: cache.(*fsStorage)}

	content, _, err := s3.Get("file/a.mjs")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(content)
	content.Close()
	if err != nil || string(data) != "data" {
		t.Fatalf("Get with a failed cache write: content %q, error %v", data, err)
	}
	if err := s3.Put("file/b.mjs", strings.NewReader("data")); err != nil || uploaded != "data" {
		t.Fatalf("Put with a failed cache write: uploaded %q, error %v", uploaded, err)
	}

	content, _, err = s3.Get("broken.mjs")
	if err != nil {
		t.Fatal(err)
	}
	data, err = io.ReadAll(content)
	content.Close()
	if !errors.Is(err, readErr) || string(data) != "da" {
		t.Fatalf("Get of a broken body: content %q, error %v", data, err)
	}
	content, _, err = s3.Get("early.mjs")
	if err != nil {
		t.Fatal(err)
	}
	content.Read(make([]byte, 1))
	content.Close()
	for _, key := range []string{"broken.mjs", "early.mjs"} {
		<-bodies["/"+key].closed
		if _, err := cache.Stat(key); !errors.Is(err, ErrNotFound) {
			t.Fatalf("partial read of %s was cached: %v", key, err)
		}
	}
}

func TestS3StorageDeleteDropsInflightCache(t *testing.T) {
	cache, err := NewFSStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	client := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = client })
	http.DefaultClient = &http.Client{Transport: s3TestTransport(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{
			"Content-Length": {"4"},
			"Last-Modified":  {time.Now().UTC().Format(http.TimeFormat)},
		}, Body: io.NopCloser(strings.NewReader("data"))}, nil
	})}
	s3 := &s3Storage{apiEndpoint: "https://storage.test", fsCache: cache.(*fsStorage)}
	content, _, err := s3.Get("modules/a.mjs")
	if err != nil {
		t.Fatal(err)
	}
	if err := s3.Delete("modules/a.mjs"); err != nil {
		t.Fatal(err)
	}
	if data, err := io.ReadAll(content); err != nil || string(data) != "data" {
		t.Fatalf("content %q, error %v", data, err)
	}
	content.Close()
	if _, err := cache.Stat("modules/a.mjs"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a read that raced with a delete was cached: %v", err)
	}
}

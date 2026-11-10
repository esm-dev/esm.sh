package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/esm-dev/esm.sh/internal/storage"
	"github.com/ije/gox/log"
)

type routerCountingStorage struct {
	storage.Storage
	mu         sync.Mutex
	gets, puts map[string]int
	delay      time.Duration
	getErr     error
}

func (s *routerCountingStorage) Get(key string) (io.ReadCloser, storage.Stat, error) {
	s.mu.Lock()
	s.gets[key]++
	s.mu.Unlock()
	time.Sleep(s.delay)
	if s.getErr != nil {
		return nil, nil, s.getErr
	}
	return s.Storage.Get(key)
}

func (s *routerCountingStorage) Put(key string, content io.Reader) error {
	return s.PutContext(context.Background(), key, content)
}

func (s *routerCountingStorage) PutContext(ctx context.Context, key string, content io.Reader) error {
	s.mu.Lock()
	s.puts[key]++
	s.mu.Unlock()
	time.Sleep(s.delay)
	return s.Storage.PutContext(ctx, key, content)
}

func TestRouterStorageRequests(t *testing.T) {
	previousConfig, previousNpmRC := config, defaultNpmRC
	testConfig := *config
	testConfig.WorkDir = t.TempDir()
	config, defaultNpmRC = &testConfig, nil
	t.Cleanup(func() { config, defaultNpmRC = previousConfig, previousNpmRC })
	logger := new(log.Logger)
	logger.SetOutput(io.Discard)
	for _, kind := range []string{"exports", "transform", "inline", "external"} {
		t.Run(kind, func(t *testing.T) {
			fs, err := storage.NewFSStorage(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			s := &routerCountingStorage{Storage: fs, gets: map[string]int{}, puts: map[string]int{}, delay: time.Second}
			method, url := "POST", "/transform"
			body, err := json.Marshal(TransformOptions{Lang: "js", Code: "export const keep = 42; export const drop = 0;", SourceMap: kind})
			if err != nil {
				t.Fatal(err)
			}
			const baseKey = "modules/storage-requests@1.0.0/es2022/storage-requests.mjs"
			if kind == "exports" {
				method, url = "GET", "/storage-requests@1.0.0/es2022/storage-requests.mjs?exports=keep"
				if err := fs.Put(baseKey, strings.NewReader("export const keep = 42; export const drop = 0;")); err != nil {
					t.Fatal(err)
				}
			}
			handler := esmRouter(s, logger)
			synctest.Test(t, func(t *testing.T) {
				responses := make([]*httptest.ResponseRecorder, 16)
				var pending sync.WaitGroup
				for i := range responses {
					pending.Go(func() {
						responses[i] = httptest.NewRecorder()
						handler.ServeHTTP(responses[i], httptest.NewRequest(method, url, bytes.NewReader(body)))
					})
				}
				pending.Wait()
				for _, res := range responses {
					if res.Code != 200 || res.Body.String() != responses[0].Body.String() || !strings.Contains(res.Body.String(), "keep") {
						t.Fatalf("inconsistent response: %d %s", res.Code, res.Body.String())
					}
					if kind == "exports" && strings.Contains(res.Body.String(), "drop") {
						t.Fatal("unused export remains in the module")
					}
				}
				wantPuts := 1
				if kind == "external" {
					wantPuts = 2
				}
				if len(s.puts) != wantPuts {
					t.Fatalf("uploaded %d objects, want %d: %v", len(s.puts), wantPuts, s.puts)
				}
				for key, n := range s.puts {
					if n != 1 {
						t.Fatalf("uploaded %s %d times, want once", key, n)
					}
				}
				if kind == "exports" && s.gets[baseKey] != 1 {
					t.Fatalf("read the base module %d times, want once", s.gets[baseKey])
				}
				for key := range s.gets {
					if kind != "external" && strings.HasSuffix(key, ".map") {
						t.Fatalf("read an unused source map: %s", key)
					}
				}
				clear(s.gets)
				res := httptest.NewRecorder()
				handler.ServeHTTP(res, httptest.NewRequest(method, url, bytes.NewReader(body)))
				reads := 0
				for _, n := range s.gets {
					reads += n
				}
				if res.Code != 200 || reads != wantPuts {
					t.Fatalf("cached request: status %d, reads %v", res.Code, s.gets)
				}
			})
		})
	}
}

func TestTransformStorageError(t *testing.T) {
	fs, err := storage.NewFSStorage(filepath.Join(t.TempDir(), "storage"))
	if err != nil {
		t.Fatal(err)
	}
	s := &routerCountingStorage{Storage: fs, gets: map[string]int{}, puts: map[string]int{}, getErr: errors.New("storage unavailable")}
	logger := new(log.Logger)
	logger.SetOutput(io.Discard)
	res := httptest.NewRecorder()
	esmRouter(s, logger).ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/transform", strings.NewReader(`{"code":"export const value = 1"}`)))
	if res.Code != 500 || len(s.puts) != 0 {
		t.Fatalf("storage error triggered generation: status %d, uploads %v", res.Code, s.puts)
	}
}

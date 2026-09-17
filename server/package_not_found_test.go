package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/esm-dev/esm.sh/internal/storage"
	"github.com/ije/gox/log"
)

func TestPackageNotFoundRoutes(t *testing.T) {
	for i, suffix := range []string{"", "/entry.js", "/index.d.ts", "/data.json", "/style.css?module", "/entry.js?raw", "/es2022/asset.svg", "?raw", "/entry.js.map"} {
		t.Run(suffix, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				useNegativeCache(t)
				previousConfig, previousNpmRC, transport := config, defaultNpmRC, http.DefaultTransport
				cfg := *config
				cfg.WorkDir = t.TempDir()
				cfg.NpmRegistry = npmRegistry
				config, defaultNpmRC = &cfg, nil
				name := fmt.Sprintf("missing-route-test-%d", i)
				t.Cleanup(func() {
					config, defaultNpmRC, http.DefaultTransport = previousConfig, previousNpmRC, transport
					deleteCacheItemsWithPrefix("npm:" + name + "@")
				})
				fs, err := storage.NewFSStorage(filepath.Join(cfg.WorkDir, "storage"))
				if err != nil {
					t.Fatal(err)
				}
				logger := new(log.Logger)
				logger.SetOutput(io.Discard)
				handler := esmRouter(fs, logger)
				requests := 0
				http.DefaultTransport = ghTestTransport(func(r *http.Request) (*http.Response, error) {
					requests++
					return &http.Response{StatusCode: 404, Body: http.NoBody}, nil
				})
				url := "http://localhost/" + name + "@1.0.0" + suffix
				cold := httptest.NewRecorder()
				handler.ServeHTTP(cold, httptest.NewRequest("GET", url, nil))
				if cold.Code != 404 || cold.Header().Get("Cache-Control") != ccTenMinutes {
					t.Errorf("cold response: HTTP %d, Cache-Control=%q: %s", cold.Code, cold.Header().Get("Cache-Control"), cold.Body.String())
				}
				unlock, err := lockInstall(context.Background(), name+"@1.0.0")
				if err != nil {
					t.Fatal(err)
				}
				done := make(chan *httptest.ResponseRecorder, 1)
				go func() {
					res := httptest.NewRecorder()
					handler.ServeHTTP(res, httptest.NewRequest("GET", url, nil))
					done <- res
				}()
				synctest.Wait()
				var cached *httptest.ResponseRecorder
				select {
				case cached = <-done:
				default:
					t.Error("cached package miss waited for the installation lock")
				}
				unlock()
				if cached == nil {
					cached = <-done
				}
				if cached.Code != 404 || cached.Header().Get("Cache-Control") != ccTenMinutes {
					t.Errorf("cached response: HTTP %d, Cache-Control=%q: %s", cached.Code, cached.Header().Get("Cache-Control"), cached.Body.String())
				}
				if requests != 1 {
					t.Errorf("cached miss fetched upstream: %d requests", requests)
				}
			})
		})
	}
}

func TestBuildQueueCachedPackageNotFound(t *testing.T) {
	for _, test := range []struct {
		name string
		pkg  npm.Package
		key  string
	}{
		{"npm package", npm.Package{Name: "missing-queue-test", Version: "1.0.0"}, "404:" + npmRegistry + "missing-queue-test@"},
		{"npm version", npm.Package{Name: "missing-queue-test", Version: "1.0.0"}, "404:" + npmRegistry + "missing-queue-test@1.0.0"},
		{"npm tarball", npm.Package{Name: "missing-queue-test", Version: "1.0.0"}, "404:" + npmRegistry + "missing-queue-test@1.0.0/install"},
		{"github", npm.Package{Github: true, Name: "owner/repo", Version: "abcdef0"}, "404:gh/owner/repo@abcdef0/install"},
		{"pkg.pr.new", npm.Package{PkgPrNew: true, Name: "pkg", Version: "abcdef0"}, "404:pr/pkg@abcdef0/install"},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				useNegativeCache(t)
				negativeCache.put(test.key, "package not found", packageNotFoundTTL)
				q := NewBuildQueue(1, time.Minute)
				release := make(chan struct{})
				blocked := newBuildQueueTestContext("/"+t.Name()+"/blocker", func() (*BuildMeta, error) {
					<-release
					return &BuildMeta{}, nil
				})
				go q.Build(context.Background(), blocked)
				synctest.Wait()
				build := newBuildQueueTestContext("/"+t.Name()+"/missing", func() (*BuildMeta, error) {
					return nil, errors.New("cached package reached build storage")
				})
				build.npmrc = &NpmRC{globalRegistry: &NpmRegistry{NpmRegistryConfig: NpmRegistryConfig{Registry: npmRegistry}}}
				build.esmPath = EsmPath{PkgName: test.pkg.Name, PkgVersion: test.pkg.Version, GhPrefix: test.pkg.Github, PrPrefix: test.pkg.PkgPrNew}
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				_, err := q.Build(ctx, build)
				if err == nil || err.Error() != "package not found" {
					t.Errorf("cached miss waited behind a build: %v", err)
				}
				if len(q.Snapshot()) != 1 {
					t.Errorf("cached miss entered the queue: %+v", q.Snapshot())
				}
				close(release)
				synctest.Wait()
			})
		})
	}
}

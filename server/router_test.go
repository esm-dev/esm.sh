package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/esm-dev/esm.sh/internal/storage"
	"github.com/ije/gox/log"
)

func TestCSSEntryRedirectURL(t *testing.T) {
	origin := "https://esm.sh"
	esmPath := EsmPath{
		PkgName:    "@material/web",
		PkgVersion: "2.4.2-nightly.95dd57c.0",
	}

	tests := []struct {
		cssEntry string
		expected string
	}{
		{
			cssEntry: "./labs/gb/components/ripple/ripple.css",
			expected: "https://esm.sh/@material/web@2.4.2-nightly.95dd57c.0/labs/gb/components/ripple/ripple.css",
		},
		{
			cssEntry: "labs/gb/components/ripple/ripple.css",
			expected: "https://esm.sh/@material/web@2.4.2-nightly.95dd57c.0/labs/gb/components/ripple/ripple.css",
		},
	}

	for _, test := range tests {
		// Calling the actual helper function from router.go
		url := getCSSEntryRedirectURL(origin, esmPath, test.cssEntry)
		if url != test.expected {
			t.Errorf("For CSSEntry %q, expected %q, but got %q", test.cssEntry, test.expected, url)
		}
	}
}

func TestGhRawAssets(t *testing.T) {
	previousConfig, previousNpmRC, transport := config, defaultNpmRC, http.DefaultTransport
	testConfig := *config
	testConfig.WorkDir = t.TempDir()
	config, defaultNpmRC = &testConfig, nil
	t.Cleanup(func() {
		config, defaultNpmRC, http.DefaultTransport = previousConfig, previousNpmRC, transport
	})
	markerDir := filepath.Join(config.WorkDir, "gh-too-large")
	if err := os.MkdirAll(markerDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(markerDir, "owner%2Frepo"), []byte("owner/repo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	fs, err := storage.NewFSStorage(filepath.Join(config.WorkDir, "storage"))
	if err != nil {
		t.Fatal(err)
	}
	logger := new(log.Logger)
	logger.SetOutput(io.Discard)
	handler := esmRouter(fs, logger)
	for _, test := range []struct {
		name        string
		path        string
		method      string
		status      int
		contentType string
		body        string
	}{
		{"svg", "assets/Alien/alien.svg", "GET", 200, "image/svg+xml; charset=utf-8", "<svg/>"},
		{"escaped path", "assets/Alien%20%231.svg", "GET", 200, "image/svg+xml; charset=utf-8", "<svg/>"},
		{"head", "alien.svg", "HEAD", 200, "image/svg+xml; charset=utf-8", "<svg/>"},
		{"conditional", "alien.svg", "GET", 304, "", ""},
		{"css", "style.css", "GET", 200, ctCSS, "body { color: red }"},
		{"json", "data.json", "GET", 200, ctJSON, "{}"},
		{"source map", "index.js.map", "GET", 200, ctJSON, "{}"},
		{"raw relative asset", "es2022/alien.svg?raw", "GET", 200, "image/svg+xml; charset=utf-8", "<svg/>"},
		{"raw typescript", "index.ts?raw", "GET", 200, ctTypeScript, "export default 1"},
		{"missing", "missing.svg", "GET", 404, "", "Not Found"},
		{"upstream error", "error.svg", "GET", 503, "", "Service Unavailable"},
		{"large content length", "large.svg", "GET", 403, "", "File Too Large"},
		{"large stream", "stream.svg", "GET", 403, "", "File Too Large"},
		{"javascript build", "index.js", "GET", 500, "", "repo is too large"},
		{"types build", "index.d.ts", "GET", 500, "", "Failed to build types: repo is too large"},
		{"json module", "data.json?module", "GET", 500, "", "repo is too large"},
		{"css module", "style.css?module", "GET", 500, "", "repo is too large"},
	} {
		t.Run(test.name, func(t *testing.T) {
			requests := 0
			http.DefaultTransport = ghTestTransport(func(r *http.Request) (*http.Response, error) {
				requests++
				if test.status == 500 || r.URL.Host != "raw.githubusercontent.com" || !strings.HasPrefix(r.URL.Path, "/owner/repo/abcdef0/") {
					t.Errorf("unexpected upstream request: %s", r.URL)
				}
				if test.name == "escaped path" && (r.URL.EscapedPath() != "/owner/repo/abcdef0/assets/Alien%20%231.svg" || r.URL.RawQuery != "") {
					t.Errorf("incorrect escaped path: %s", r.URL)
				}
				if test.name == "raw relative asset" && r.URL.Path != "/owner/repo/abcdef0/alien.svg" {
					t.Errorf("incorrect relative asset path: %s", r.URL)
				}
				if deadline, ok := r.Context().Deadline(); !ok || time.Until(deadline) > 30*time.Second {
					t.Error("expected a download deadline")
				}
				res := &http.Response{
					StatusCode:    200,
					Header:        http.Header{"Content-Type": {"text/plain"}, "Etag": {`"asset"`}},
					ContentLength: int64(len(test.body)),
					Body:          io.NopCloser(strings.NewReader(test.body)),
				}
				switch test.name {
				case "conditional":
					if r.Header.Get("If-None-Match") != `"asset"` {
						t.Error("conditional header was not forwarded")
					}
					res.StatusCode = 304
				case "missing", "upstream error":
					res.StatusCode = test.status
				case "large content length":
					res.ContentLength = maxAssetFileSize + 1
				case "large stream":
					res.ContentLength = -1
					res.Body = io.NopCloser(io.LimitReader(zeroReader{}, maxAssetFileSize+1))
				}
				return res, nil
			})
			req := httptest.NewRequest(test.method, "http://localhost/gh/owner/repo@abcdef0/"+test.path, nil)
			if test.name == "conditional" {
				req.Header.Set("If-None-Match", `"asset"`)
			}
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			if res.Code != test.status {
				t.Fatalf("expected status %d, got %d: %s", test.status, res.Code, res.Body.String())
			}
			if test.contentType != "" && res.Header().Get("Content-Type") != test.contentType {
				t.Fatalf("unexpected content type: %s", res.Header().Get("Content-Type"))
			}
			if test.status == 200 || test.status == 304 {
				if res.Header().Get("Cache-Control") != ccImmutable || res.Header().Get("Etag") != `"asset"` {
					t.Fatalf("missing cache headers: %v", res.Header())
				}
			}
			body := test.body
			if test.method == "HEAD" {
				body = ""
			}
			if res.Body.String() != body {
				t.Fatalf("unexpected response body: %q", res.Body.String())
			}
			if test.status == 500 && requests != 0 || test.status != 500 && requests != 1 {
				t.Fatalf("unexpected upstream request count: %d", requests)
			}
		})
	}
	http.DefaultTransport = ghTestTransport(func(r *http.Request) (*http.Response, error) {
		t.Errorf("redirect made an upstream request: %s", r.URL)
		return &http.Response{StatusCode: 404, Body: http.NoBody}, nil
	})
	for _, test := range []struct{ path, location string }{
		{"es2022/alien.svg", "alien.svg"},
		{"es2022/napi/parser/parser.wasm32-wasi.wasm", "napi/parser/parser.wasm32-wasi.wasm"},
		{"X-ZHJlYWN0QDE4LjMuMQ/es2022/napi/parser/parser.wasm32-wasi.wasm", "napi/parser/parser.wasm32-wasi.wasm"},
		{"es2022/assets/Alien%20%231.svg?key=a%2Bb", "assets/Alien%20%231.svg?key=a%2Bb"},
		{"es2022/data.json?module", "data.json?module"},
	} {
		t.Run("redirect/"+test.path, func(t *testing.T) {
			base := "http://localhost/gh/owner/repo@abcdef0/"
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, httptest.NewRequest("GET", base+test.path, nil))
			if res.Code != 301 || res.Header().Get("Location") != base+test.location {
				t.Fatalf("unexpected redirect: %d %s", res.Code, res.Header().Get("Location"))
			}
			if res.Header().Get("Cache-Control") != ccImmutable {
				t.Fatal("redirect is not immutable")
			}
		})
	}
	if existsDir(filepath.Join(config.WorkDir, "npm", "gh", "owner")) {
		t.Fatal("asset request installed the repository")
	}
}

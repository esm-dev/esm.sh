package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/esm-dev/esm.sh/internal/storage"
	"github.com/ije/gox/log"
)

func TestRouterLicenseFiles(t *testing.T) {
	previousConfig, previousNpmRC, transport := config, defaultNpmRC, http.DefaultTransport
	testConfig := *config
	testConfig.WorkDir = t.TempDir()
	config, defaultNpmRC = &testConfig, nil
	t.Cleanup(func() {
		config, defaultNpmRC, http.DefaultTransport = previousConfig, previousNpmRC, transport
	})
	files := []string{"COPYING", "COPYING.LESSER", "LICENSE", "LICENCE", "COPYRIGHT", "NOTICE", "UNLICENSE", "nested/license", "LICENSE.md"}
	const notice = "<Copyright> Example authors\nAll rights reserved.\n"
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	for _, name := range append(files, "package.json", "unsupported", "LICENSE.js") {
		content := notice
		if name == "package.json" {
			content = `{"name":"license-files-test","version":"1.0.0"}`
		}
		if name == "LICENSE.js" {
			content = "export default 42"
		}
		if err := tw.WriteHeader(&tar.Header{Name: "package/" + name, Mode: 0644, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(tw, content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	setCacheItem("npm:license-files-test@1.0.0", &npm.PackageJSON{
		Name: "license-files-test", Version: "1.0.0",
		Dist: npm.NpmPackageDist{Tarball: "https://registry.example/license-files-test.tgz"},
	}, time.Minute)
	t.Cleanup(func() { deleteCacheItem("npm:license-files-test@1.0.0") })
	downloads := 0
	http.DefaultTransport = ghTestTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://registry.example/license-files-test.tgz" {
			t.Fatalf("unexpected request: %s", r.URL)
		}
		downloads++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(archive.Bytes()))}, nil
	})
	fs, err := storage.NewFSStorage(filepath.Join(config.WorkDir, "storage"))
	if err != nil {
		t.Fatal(err)
	}
	logger := new(log.Logger)
	logger.SetOutput(io.Discard)
	handler := esmRouter(fs, logger)
	for _, file := range files {
		for _, query := range []string{"?raw", ""} {
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, httptest.NewRequest("GET", "http://localhost/license-files-test@1.0.0/"+file+query, nil))
			if res.Code != http.StatusOK || res.Body.String() != notice {
				t.Fatalf("%s%s: HTTP %d: %s", file, query, res.Code, res.Body.String())
			}
			contentType := "text/plain; charset=utf-8"
			if strings.HasSuffix(file, ".md") {
				contentType = "text/markdown; charset=utf-8"
			}
			if got := res.Header().Get("Content-Type"); got != contentType {
				t.Errorf("%s: Content-Type = %q, want %q", file, got, contentType)
			}
		}
	}
	if downloads != 1 {
		t.Errorf("got %d downloads, want 1", downloads)
	}
	build := &BuildContext{esmPath: EsmPath{PkgName: "license-files-test", PkgVersion: "1.0.0", SubPath: "LICENSE"}, target: "es2022"}
	t.Cleanup(func() { cacheLRU.Remove(build.Path()) })
	if err := NewBuildMetaDB(fs).Put(build.Path(), encodeBuildMeta(&BuildMeta{ExportDefault: true})); err != nil {
		t.Fatal(err)
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest("GET", "http://localhost/license-files-test@1.0.0/LICENSE.js?target=es2022", nil))
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), build.Path()) {
		t.Errorf("LICENSE.js: HTTP %d: %s", res.Code, res.Body.String())
	}
	if _, err := os.Stat(filepath.Join(config.WorkDir, "npm", "license-files-test@1.0.0", "node_modules", "license-files-test", "unsupported")); !os.IsNotExist(err) {
		t.Errorf("unsupported file: %v", err)
	}
}

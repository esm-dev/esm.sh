package server

import (
	"fmt"
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

func TestRouterNoDtsHeaderOnly(t *testing.T) {
	previousConfig, previousNpmRC, transport := config, defaultNpmRC, http.DefaultTransport
	testConfig := *config
	testConfig.WorkDir = t.TempDir()
	config, defaultNpmRC = &testConfig, nil
	t.Cleanup(func() {
		config, defaultNpmRC, http.DefaultTransport = previousConfig, previousNpmRC, transport
		for _, key := range cacheLRU.Keys() {
			if strings.HasPrefix(key, "/no-dts-") {
				cacheLRU.Remove(key)
			}
		}
	})
	http.DefaultTransport = ghTestTransport(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected request: %s", r.URL)
		return &http.Response{StatusCode: 500, Body: http.NoBody}, nil
	})
	for _, name := range []string{"no-dts-root", "no-dts-dep"} {
		pkgDir := filepath.Join(config.WorkDir, "npm", name+"@1.0.0", "node_modules", name)
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			t.Fatal(err)
		}
		code := `export const value = 42;`
		if name == "no-dts-root" {
			code = `export { value } from "no-dts-dep";`
		}
		for filename, content := range map[string]string{
			"package.json": fmt.Sprintf(`{"name":%q,"version":"1.0.0","type":"module","main":"index.js","types":"index.d.ts","dependencies":{"no-dts-dep":"^1.0.0"}}`, name),
			"index.js":     code,
			"index.d.ts":   `export declare const value: number;`,
		} {
			if err := os.WriteFile(filepath.Join(pkgDir, filename), []byte(content), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	setCacheItem("npm:no-dts-dep@^1.0.0", &npm.PackageJSON{Name: "no-dts-dep", Version: "1.0.0"}, time.Minute)
	t.Cleanup(func() { deleteCacheItem("npm:no-dts-dep@^1.0.0") })
	fs, err := storage.NewFSStorage(filepath.Join(config.WorkDir, "storage"))
	if err != nil {
		t.Fatal(err)
	}
	build := &BuildContext{esmPath: EsmPath{PkgName: "no-dts-types", PkgVersion: "1.0.0"}, target: "es2022"}
	if err := NewBuildMetaDB(fs).Put(build.Path(), encodeBuildMeta(&BuildMeta{TypesOnly: true, Dts: "/no-dts-types@1.0.0/index.d.ts"})); err != nil {
		t.Fatal(err)
	}
	logger := new(log.Logger)
	logger.SetOutput(io.Discard)
	handler := esmRouter(fs, logger)
	get := func(url string) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest("GET", "http://localhost"+url, nil))
		return res
	}

	for _, url := range []string{"/no-dts-types@1.0.0?target=es2022", "/no-dts-root@1.0.0?target=es2022&no-bundle", "/no-dts-root@1.0.0/es2022/no-dts-root.nobundle.mjs"} {
		want := get(url)
		if want.Code != 200 {
			t.Fatalf("%s: HTTP %d: %s", url, want.Code, want.Body.String())
		}
		if strings.Contains(url, "?target=") && want.Header().Get("X-TypeScript-Types") == "" {
			t.Fatalf("%s: missing type header", url)
		}
		sep := "?"
		if strings.Contains(url, "?") {
			sep = "&"
		}
		for _, flag := range []string{"no-dts", "no-check"} {
			// the same module must be served, otherwise mixing flagged and plain imports duplicates it
			res := get(url + sep + flag)
			if res.Code != 200 || res.Body.String() != want.Body.String() {
				t.Fatalf("%s%s%s: HTTP %d, module changed:\n%s\nwant:\n%s", url, sep, flag, res.Code, res.Body.String(), want.Body.String())
			}
			if dts := res.Header().Get("X-TypeScript-Types"); dts != "" {
				t.Fatalf("%s%s%s: unexpected type header: %s", url, sep, flag, dts)
			}
		}
	}
}

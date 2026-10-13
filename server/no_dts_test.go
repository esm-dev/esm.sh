package server

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/esm-dev/esm.sh/internal/storage"
	"github.com/ije/gox/log"
)

func TestRouterNoDtsDependencies(t *testing.T) {
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
	for _, name := range []string{"no-dts-root", "no-dts-dep", "no-dts-leaf"} {
		pkgDir := filepath.Join(config.WorkDir, "npm", name+"@1.0.0", "node_modules", name)
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			t.Fatal(err)
		}
		code := `export const value = 42;`
		if name == "no-dts-root" {
			code = `export { value } from "no-dts-dep"; export const load = () => import("no-dts-root/lazy");`
		} else if name == "no-dts-dep" {
			code = `export { value } from "no-dts-leaf";`
		}
		for filename, content := range map[string]string{
			"package.json": fmt.Sprintf(`{"name":%q,"version":"1.0.0","type":"module","main":"index.js","types":"index.d.ts","dependencies":{"no-dts-dep":"^1.0.0","no-dts-leaf":"1.0.0"}}`, name),
			"index.js":     code, "lazy.js": `export { value as lazy } from "no-dts-dep";`,
			"index.d.ts": `export declare const value: number;`, "lazy.d.ts": `export declare const lazy: number;`,
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
	logger := new(log.Logger)
	logger.SetOutput(io.Discard)
	handler := esmRouter(fs, logger)
	for _, noDts := range []bool{false, true} {
		build := &BuildContext{esmPath: EsmPath{PkgName: "no-dts-types", PkgVersion: "1.0.0"}, target: "es2022", args: BuildArgs{NoDts: noDts}}
		if err := NewBuildMetaDB(fs).Put(build.Path(), encodeBuildMeta(&BuildMeta{TypesOnly: true, Dts: "/no-dts-types@1.0.0/index.d.ts"})); err != nil {
			t.Fatal(err)
		}
	}
	if err := fs.Put("modules/no-dts-root@1.0.0/es2022/no-dts-root.css", strings.NewReader("body{color:red}")); err != nil {
		t.Fatal(err)
	}
	imports := regexp.MustCompile(`"([./][^"\n]+)"`)
	for _, flag := range []string{"", "no-check", "no-dts", "no-check", ""} {
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest("GET", "http://localhost/no-dts-types@1.0.0?target=es2022&"+flag, nil))
		if res.Code != 200 || (res.Header().Get("X-TypeScript-Types") != "") != (flag == "") {
			t.Fatalf("types-only package with %q: HTTP %d, headers: %v", flag, res.Code, res.Header())
		}
		res = httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest("GET", "http://localhost/no-dts-root@1.0.0/es2022/no-dts-root.css?"+flag, nil))
		if res.Code != 200 || res.Body.String() != "body{color:red}" {
			t.Fatalf("CSS with %q: HTTP %d: %s", flag, res.Code, res.Body.String())
		}
		for _, entry := range []string{"/no-dts-root@1.0.0?target=es2022&no-bundle&", "/no-dts-root@1.0.0/es2022/no-dts-root.nobundle.mjs?"} {
			t.Run(entry+flag, func(t *testing.T) {
				pending := []string{entry + flag}
				seen := map[string]bool{}
				for len(pending) > 0 {
					url := pending[0]
					pending = pending[1:]
					if seen[url] {
						continue
					}
					seen[url] = true
					res := httptest.NewRecorder()
					req := httptest.NewRequest("GET", "http://localhost"+url, nil)
					handler.ServeHTTP(res, req)
					if res.Code == http.StatusMovedPermanently || res.Code == http.StatusFound {
						pending = append(pending, strings.TrimPrefix(res.Header().Get("Location"), "http://localhost"))
						continue
					}
					if res.Code != 200 {
						t.Fatalf("%s: HTTP %d: %s", url, res.Code, res.Body.String())
					}
					if dts := res.Header().Get("X-TypeScript-Types"); flag != "" && dts != "" {
						t.Fatalf("%s: unexpected type header: %s", url, dts)
					} else if flag == "" && strings.Contains(url, "?target=") && dts == "" {
						t.Fatalf("%s: missing type header", url)
					}
					for _, match := range imports.FindAllStringSubmatch(res.Body.String(), -1) {
						dep, err := req.URL.Parse(match[1])
						if err != nil {
							t.Fatal(err)
						}
						pending = append(pending, dep.RequestURI())
					}
				}
				if len(seen) < 5 {
					t.Fatalf("incomplete module graph: %v", seen)
				}
			})
		}
	}
}

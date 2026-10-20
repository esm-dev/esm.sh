package importmap

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseEsmPath(t *testing.T) {
	for _, test := range []struct {
		path string
		want Import
	}{
		{"/pkg@1.0.0/es2022/pkg.mjs", Import{Name: "pkg", Version: "1.0.0"}},
		{"/*pkg@1.0.0/es2022/sub.mjs", Import{Name: "pkg", Version: "1.0.0", SubPath: "sub"}},
		{"/@scope/pkg@1.0.0/es2022/pkg.mjs", Import{Name: "@scope/pkg", Version: "1.0.0"}},
		{"/*@scope/pkg@1.0.0/es2022/pkg.mjs", Import{Name: "@scope/pkg", Version: "1.0.0"}},
		{"/*@scope/pkg@1.0.0/es2022/sub.development.mjs", Import{Name: "@scope/pkg", Version: "1.0.0", SubPath: "sub", Dev: true}},
		{"/*@scope/pkg/sub/file.js", Import{Name: "@scope/pkg", SubPath: "sub/file.js"}},
		{"/gh/owner/repo@1.0.0", Import{Name: "owner/repo", Version: "1.0.0", Github: true}},
		{"/gh/owner/repo@1.0.0/es2022/repo.mjs", Import{Name: "owner/repo", Version: "1.0.0", Github: true}},
		{"/gh/owner/repo@main/src/index.js", Import{Name: "owner/repo", Version: "main", SubPath: "src/index.js", Github: true}},
		{"/gh/owner/repo/es2022/sub/file.mjs", Import{Name: "owner/repo", SubPath: "sub/file", Github: true}},
		{"/gh/*owner/repo@1.0.0/es2022/repo.development.mjs", Import{Name: "owner/repo", Version: "1.0.0", Github: true, Dev: true}},
		{"/*gh/owner/repo@1.0.0/es2022/sub.mjs", Import{Name: "owner/repo", Version: "1.0.0", SubPath: "sub", Github: true}},
		{"/jsr/@scope/pkg@1.0.0/es2022/scope__pkg.mjs", Import{Name: "@scope/pkg", Version: "1.0.0", Jsr: true}},
		{"/jsr/*@scope/pkg@1.0.0/es2022/sub.mjs", Import{Name: "@scope/pkg", Version: "1.0.0", SubPath: "sub", Jsr: true}},
		{"/*jsr/@scope/pkg@1.0.0/es2022/scope__pkg.mjs", Import{Name: "@scope/pkg", Version: "1.0.0", Jsr: true}},
	} {
		for _, prefix := range []string{"", "https://esm.sh", "http://localhost:8080"} {
			path := prefix + test.path + "?target=es2022#module"
			t.Run(path, func(t *testing.T) {
				got, err := ParseEsmPath(path)
				if err != nil || got != test.want {
					t.Fatalf("ParseEsmPath(%q) = %+v, %v; want %+v", path, got, err, test.want)
				}
			})
		}
	}
}

func TestParseEsmPathInvalid(t *testing.T) {
	for _, path := range []string{"/", "/*", "/@scope", "/*@scope", "/*@scope/", "/gh/", "/gh/owner", "/gh/owner/", "/gh/*owner", "/gh/*owner/", "/*gh/owner/"} {
		t.Run(path, func(t *testing.T) {
			if got, err := ParseEsmPath(path); err == nil {
				t.Fatalf("ParseEsmPath(%q) = %+v; want an error", path, got)
			}
		})
	}
}

func TestAddJsrImport(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jsr/@std/path@1.0.8" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"name":"@jsr/std__path","version":"1.0.8","integrity":"sha384-path","imports":["/@jsr/std__path@1.0.8/es2022/basename.mjs"]}`)
	}))
	defer cdn.Close()
	im := Blank()
	im.SetConfig(Config{CDN: cdn.URL})
	if _, errors := im.AddImportFromSpecifier("jsr:@std/path@1.0.8", false); len(errors) > 0 {
		t.Fatal(errors)
	}
	want := cdn.URL + "/jsr/@std/path@1.0.8/es2022/std__path.mjs"
	if got, _ := im.Imports.Get("jsr:@std/path"); got != want {
		t.Fatalf("jsr:@std/path = %q, want %q (keys %v)", got, want, im.Imports.Keys())
	}
	if got, _ := im.Integrity().Get(want); got != "sha384-path" {
		t.Fatalf("integrity = %q", got)
	}
	if imp, err := ParseEsmPath(want); err != nil || imp.Specifier(false) != "jsr:@std/path" {
		t.Fatalf("ParseEsmPath(%q) = %+v, %v", want, imp, err)
	}
	meta := ImportMeta{Import: Import{Name: "@std/path", Version: "1.0.8", Jsr: true}, Imports: []string{"/@jsr/std__fmt@1.0.0/es2022/colors.mjs"}}
	if got := meta.EsmSpecifier(); got != "*jsr/@std/path@1.0.8" {
		t.Fatalf("EsmSpecifier() = %q", got)
	}
}

func TestFetchImportMetaConcurrency(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var mu sync.Mutex
	var requests, inflight, maxInflight int
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests, inflight = requests+1, inflight+1
		maxInflight = max(maxInflight, inflight)
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		inflight--
		mu.Unlock()
		name := strings.TrimPrefix(r.URL.Path, "/")
		fmt.Fprintf(w, `{"name":%q,"version":"1.0.0"}`, name)
	}))
	defer cdn.Close()
	var wg sync.WaitGroup
	for i := range 64 {
		wg.Go(func() {
			if _, err := fetchImportMeta(cdn.URL, Import{Name: fmt.Sprintf("pkg-%d", i%32)}, ""); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if requests != 32 || maxInflight > cap(fetchLimit) {
		t.Errorf("got %d requests (want 32), %d concurrent (want at most %d)", requests, maxInflight, cap(fetchLimit))
	}
}

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/esm-dev/esm.sh/internal/importmap"
)

func TestTidyPreservesUnmanagedScopes(t *testing.T) {
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"name":"managed","version":"1.2.3"}`)
	}))
	defer cdn.Close()
	for _, tc := range []struct{ specifier, path string }{
		{"alias", "/legacy@1.2.3/es2022/legacy.mjs"},
		{"legacy", "/legacy@1/es2022/legacy.mjs"},
		{"", ""},
	} {
		t.Run(tc.specifier, func(t *testing.T) {
			t.Chdir(t.TempDir())
			im := importmap.ImportMapJson{
				Config:  importmap.Config{CDN: cdn.URL},
				Imports: map[string]string{"managed": cdn.URL + "/managed@1.2.3/es2022/managed.mjs"},
				Scopes: map[string]map[string]string{
					cdn.URL + "/":              {"shared": "/shared.js"},
					cdn.URL + "/legacy@1.2.3/": {"dep": cdn.URL + "/dep@1.0.0/es2022/dep.mjs"},
					cdn.URL + "/dep@1.0.0/":    {"custom": "/custom.js"},
					"/app/":                    {"local": "/local.js"},
				},
			}
			if tc.specifier != "" {
				im.Imports[tc.specifier] = cdn.URL + tc.path
			}
			data, err := json.Marshal(im)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile("index.html", []byte(`<script type="importmap">`+string(data)+`</script>`), 0644); err != nil {
				t.Fatal(err)
			}
			if err := tidy(true); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile("index.html")
			if err != nil {
				t.Fatal(err)
			}
			_, raw, _ := strings.Cut(string(got), `<script type="importmap">`)
			raw, _, _ = strings.Cut(raw, "</script>")
			var result importmap.ImportMapJson
			if err := json.Unmarshal([]byte(raw), &result); err != nil {
				t.Fatal(err)
			}
			if tc.specifier != "" {
				if result.Imports[tc.specifier] != im.Imports[tc.specifier] {
					t.Fatal("unmanaged import changed")
				}
				for scope, imports := range im.Scopes {
					for specifier, url := range imports {
						if result.Scopes[scope][specifier] != url {
							t.Errorf("lost scoped import %q in %q", specifier, scope)
						}
					}
				}
			} else if len(result.Scopes) != 1 || result.Scopes["/app/"]["local"] != "/local.js" {
				t.Fatalf("stale CDN scopes were not removed: %v", result.Scopes)
			}
		})
	}
}

func TestTidyPreservesDevelopmentBuilds(t *testing.T) {
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		imp, err := importmap.ParseSpecifier(strings.TrimPrefix(r.URL.Path, "/"))
		if err != nil {
			t.Error(err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		integrity := "sha384-production"
		if r.URL.Query().Has("dev") {
			integrity = "sha384-development"
		}
		json.NewEncoder(w).Encode(importmap.ImportMeta{Import: imp, Integrity: integrity})
	}))
	defer cdn.Close()
	for _, tc := range []struct{ name, subpath, module string }{
		{"pkg", "", "pkg"},
		{"pkg", "client", "client"},
		{"@scope/pkg", "", "pkg"},
	} {
		t.Run(tc.name+"/"+tc.subpath, func(t *testing.T) {
			t.Chdir(t.TempDir())
			imp := importmap.Import{Name: tc.name, Version: "1.2.3", SubPath: tc.subpath}
			im := importmap.Blank()
			im.SetConfig(importmap.Config{CDN: cdn.URL})
			if _, err := im.FetchImportMeta(imp); err != nil {
				t.Fatal(err)
			}
			url := cdn.URL + "/" + tc.name + "@1.2.3/es2022/" + tc.module + ".development.mjs"
			source := fmt.Sprintf(`<script type="importmap">{"config":{"cdn":%q},"imports":{%q:%q}}</script>`, cdn.URL, imp.Specifier(false), url)
			if err := os.WriteFile("index.html", []byte(source), 0644); err != nil {
				t.Fatal(err)
			}
			if err := tidy(false); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile("index.html")
			if err != nil {
				t.Fatal(err)
			}
			_, raw, _ := strings.Cut(string(got), `<script type="importmap">`)
			raw, _, _ = strings.Cut(raw, "</script>")
			var result importmap.ImportMapJson
			if err := json.Unmarshal([]byte(raw), &result); err != nil {
				t.Fatal(err)
			}
			if result.Imports[imp.Specifier(false)] != url || result.Integrity[url] != "sha384-development" {
				t.Fatalf("incorrect development build or integrity: %s", got)
			}
		})
	}
}

func TestTidyFailurePreservesHTML(t *testing.T) {
	t.Chdir(t.TempDir())
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer cdn.Close()
	source := fmt.Sprintf(`<head><script type="importmap">{"config":{"cdn":%q},"imports":{"react":%q}}</script></head>`, cdn.URL, cdn.URL+"/react@19.0.0/es2022/react.mjs")
	if err := os.WriteFile("index.html", []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	if err := tidy(true); err == nil {
		t.Fatal("expected resolution error")
	}
	got, err := os.ReadFile("index.html")
	if err != nil || string(got) != source {
		t.Fatalf("index.html changed on failure: %q, %v", got, err)
	}
}

func TestTidyPreservesUnmanagedImports(t *testing.T) {
	for _, imports := range []string{
		`"react":"https://other.example/react@19.0.0/es2022/react.mjs"`,
		`"custom":"https://esm.sh/react@19.0.0/es2022/react.mjs"`,
		`"react":"https://esm.sh/react@19"`,
	} {
		t.Run(imports, func(t *testing.T) {
			t.Chdir(t.TempDir())
			source := `<head><script type="importmap">{"imports":{` + imports + `}}</script></head>`
			if err := os.WriteFile("index.html", []byte(source), 0644); err != nil {
				t.Fatal(err)
			}
			if err := tidy(true); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile("index.html")
			if err != nil || string(got) != source {
				t.Fatalf("unmanaged imports changed: %q, %v", got, err)
			}
		})
	}
}

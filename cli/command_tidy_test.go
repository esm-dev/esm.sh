package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

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

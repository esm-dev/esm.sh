package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/esm-dev/esm.sh/internal/importmap"
)

type cliTestTransport func(*http.Request) (*http.Response, error)

func (f cliTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestUpdateImportMapStarter(t *testing.T) {
	for _, tc := range []struct{ input, name, specifier string }{
		{"cli-test-starter@1", "cli-test-starter", "cli-test-starter"},
		{"@cli-test/starter@1.2.3/client", "@cli-test/starter", "@cli-test/starter/client"},
		{"gh:cli-test/starter@v1.2.3", "cli-test/starter", "gh:cli-test/starter"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			t.Chdir(t.TempDir())
			transport := http.DefaultTransport
			t.Cleanup(func() { http.DefaultTransport = transport })
			http.DefaultTransport = cliTestTransport(func(r *http.Request) (*http.Response, error) {
				body := fmt.Sprintf(`{"name":%q,"version":"1.2.3"}`, tc.name)
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})
			if err := updateImportMap([]string{tc.input}, false, true, true, false); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile("index.html")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(got), fmt.Sprintf("import * as mod from %q;", tc.specifier)) || !strings.Contains(string(got), fmt.Sprintf("%q:", tc.specifier)) {
				t.Fatalf("starter import is not mapped: %s", got)
			}
		})
	}
}

func TestUpdateImportMapAfterClassicScript(t *testing.T) {
	t.Chdir(t.TempDir())
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cli-test-config" || r.URL.Query().Get("target") != "es2020" {
			t.Errorf("unexpected metadata URL: %s", r.URL)
		}
		io.WriteString(w, `{"name":"cli-test-config","version":"1.2.3"}`)
	}))
	defer cdn.Close()
	for _, prefix := range []string{
		`<head><script src="/classic.js"></script>`,
		`<head><script>console.log("hello")</script>`,
		`<head></head><body>`,
	} {
		source := prefix + fmt.Sprintf(`<script type="importmap">{"config":{"cdn":%q,"target":"es2020"},"imports":{"local":"./local.js"}}</script>`, cdn.URL)
		if err := os.WriteFile("index.html", []byte(source), 0644); err != nil {
			t.Fatal(err)
		}
		if err := updateImportMap([]string{"cli-test-config"}, false, true, true, false); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile("index.html")
		if err != nil {
			t.Fatal(err)
		}
		text := string(got)
		if !strings.HasPrefix(text, prefix) || strings.Count(text, `type="importmap"`) != 1 {
			t.Fatalf("changed script structure: %s", got)
		}
		_, raw, _ := strings.Cut(text, `<script type="importmap">`)
		raw, _, _ = strings.Cut(raw, "</script>")
		var im importmap.ImportMapJson
		if err := json.Unmarshal([]byte(raw), &im); err != nil {
			t.Fatal(err)
		}
		if im.Imports["local"] != "./local.js" || im.Imports["cli-test-config"] != cdn.URL+"/cli-test-config@1.2.3/es2020/cli-test-config.mjs" {
			t.Fatalf("incorrect imports: %v", im.Imports)
		}
	}
}

func TestUpdateImportMapFailurePreservesHTML(t *testing.T) {
	transport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = transport })
	http.DefaultTransport = cliTestTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader("unavailable")), Header: make(http.Header)}, nil
	})
	for _, source := range []string{
		"<html><head></head><body></body></html>",
		`<html><head><script type="module"></script></head></html>`,
		`<html><head><script type="importmap">{"imports":{}}</script></head></html>`,
	} {
		t.Run(source, func(t *testing.T) {
			t.Chdir(t.TempDir())
			if err := os.WriteFile("index.html", []byte(source), 0644); err != nil {
				t.Fatal(err)
			}
			if err := updateImportMap([]string{"cli-test-missing"}, false, true, true, false); err == nil {
				t.Fatal("expected resolution error")
			}
			got, err := os.ReadFile("index.html")
			if err != nil || string(got) != source {
				t.Fatalf("index.html changed on failure: %q, %v", got, err)
			}
		})
	}
}

func TestUpdateImportMapWithoutScript(t *testing.T) {
	for _, source := range []string{
		"<html><head></head></html>",
		`<head><script type="module">import "pkg"</script></head>`,
		`<head><script type="importmap"></script></head>`,
	} {
		t.Run(source, func(t *testing.T) {
			t.Chdir(t.TempDir())
			if err := os.WriteFile("index.html", []byte(source), 0644); err != nil {
				t.Fatal(err)
			}
			if err := updateImportMap(nil, false, true, true, false); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile("index.html")
			if err != nil || !strings.Contains(string(got), "\"imports\": {}") || strings.Count(string(got), "</script>") != strings.Count(source, "</script>")+1-strings.Count(source, `type="importmap"`) {
				t.Fatalf("invalid HTML: %q, %v", got, err)
			}
			if strings.Contains(source, `type="module"`) && strings.Index(string(got), `type="importmap"`) > strings.Index(string(got), `type="module"`) {
				t.Fatalf("import map follows the module script: %s", got)
			}
		})
	}
}

func TestAddImportsConcurrentErrors(t *testing.T) {
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer cdn.Close()
	im := importmap.Blank()
	im.SetConfig(importmap.Config{CDN: cdn.URL})
	imports := make([]importmap.Import, 64)
	for i := range imports {
		imports[i] = importmap.Import{Name: fmt.Sprintf("missing-%d", i)}
	}
	if addImports(im, imports, false, true, true, "") {
		t.Fatal("invalid imports succeeded")
	}
}

func TestLookupClosestFileStatError(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.Symlink("index.html", "index.html"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := lookupClosestFile("index.html"); err == nil {
		t.Fatal("expected symlink loop error")
	}
}

func TestLookupClosestFileParent(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "index.html")
	if err := os.WriteFile(filename, nil, 0644); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(dir, "child")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(child)
	got, exists, err := lookupClosestFile("index.html")
	if err != nil || !exists || got != filename {
		t.Fatalf("lookup = %q, %v, %v", got, exists, err)
	}
}

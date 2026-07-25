package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/esm-dev/esm.sh/internal/storage"
	"github.com/ije/rex"
)

func TestWorkerFactoryEscapesModuleURL(t *testing.T) {
	moduleUrl := "https://esm.sh\" } = options; globalThis.PWNED=1; const { z = \"\\\r\n\u2028\u2029"
	code := newWorkerFactory(moduleUrl)

	const urlPrefix = "const moduleUrl = "
	start := strings.Index(code, urlPrefix)
	if start < 0 {
		t.Fatal("missing module URL")
	}
	start += len(urlPrefix)
	end := strings.Index(code[start:], "; const options")
	if end < 0 {
		t.Fatal("invalid worker factory")
	}
	var gotUrl string
	if err := json.Unmarshal([]byte(code[start:start+end]), &gotUrl); err != nil {
		t.Fatal(err)
	}
	if gotUrl != moduleUrl {
		t.Fatalf("module URL changed: %q", gotUrl)
	}

	const importPrefix = "const blob = new Blob(["
	start = strings.Index(code, importPrefix)
	if start < 0 {
		t.Fatal("missing worker import")
	}
	start += len(importPrefix)
	end = strings.Index(code[start:], ", inject]")
	if end < 0 {
		t.Fatal("invalid worker import")
	}
	var gotImport string
	if err := json.Unmarshal([]byte(code[start:start+end]), &gotImport); err != nil {
		t.Fatal(err)
	}
	urlLiteral, _ := json.Marshal(moduleUrl)
	wantImport := "import * as $module from " + string(urlLiteral) + ";"
	if gotImport != wantImport {
		t.Fatalf("unexpected worker import %q", gotImport)
	}
}

func TestJSONModuleRoute(t *testing.T) {
	root := t.TempDir()
	oldConfig := config
	config = DefaultConfig()
	config.WorkDir = root
	defer func() {
		config = oldConfig
	}()

	pkgDir := filepath.Join(root, "npm", "json-pkg@1.0.0", "node_modules", "json-pkg")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(pkgDir, "payload.json")
	if err := os.WriteFile(filename, []byte(`{};globalThis.PWNED=1`), 0644); err != nil {
		t.Fatal(err)
	}
	fs, err := storage.NewFSStorage(filepath.Join(root, "storage"))
	if err != nil {
		t.Fatal(err)
	}
	mux := rex.New()
	mux.Use(esmRouter(fs, nil))

	request := func(ifNoneMatch string) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/json-pkg@1.0.0/payload.json?module", nil)
		if ifNoneMatch != "" {
			req.Header.Set("If-None-Match", ifNoneMatch)
		}
		mux.ServeHTTP(res, req)
		return res
	}
	stat, err := os.Stat(filename)
	if err != nil {
		t.Fatal(err)
	}
	etag := fmt.Sprintf(`W/"%x-%x"`, stat.ModTime().Unix(), stat.Size())
	res := request(etag)
	if res.Code != http.StatusInternalServerError {
		t.Fatalf("expected invalid JSON to return 500, got %d: %s", res.Code, res.Body)
	}
	if res.Header().Get("Cache-Control") != ccMustRevalidate {
		t.Fatalf("invalid JSON must not be immutable: %q", res.Header().Get("Cache-Control"))
	}
	if res.Header().Get("Etag") != "" || res.Header().Get("Last-Modified") != "" {
		t.Fatal("invalid JSON must not return cache validators")
	}

	if err := os.WriteFile(filename, []byte(`{"html":"</script>"}`), 0644); err != nil {
		t.Fatal(err)
	}
	res = request("")
	if res.Code != http.StatusOK {
		t.Fatalf("expected valid JSON to return 200, got %d: %s", res.Code, res.Body)
	}
	if strings.Contains(res.Body.String(), "</script>") {
		t.Fatalf("unsafe JSON module: %s", res.Body)
	}
	if res.Header().Get("Content-Length") != strconv.Itoa(res.Body.Len()) {
		t.Fatalf("invalid Content-Length %q for %d-byte body", res.Header().Get("Content-Length"), res.Body.Len())
	}
}

func TestIndexOriginIsConfigured(t *testing.T) {
	t.Chdir("..")
	cacheStore.Delete("index.html")
	defer cacheStore.Delete("index.html")

	oldConfig := config
	config = DefaultConfig()
	config.CdnOrigin = "https://cdn.example.com"
	defer func() {
		config = oldConfig
	}()

	fs, err := storage.NewFSStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mux := rex.New()
	mux.Use(esmRouter(fs, nil))
	request := func(host string, poisoned bool) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://"+host+"/", nil)
		req.Header.Set("User-Agent", "Mozilla/5.0")
		if poisoned {
			req.Header.Set("X-Real-Origin", `https://esm.sh";globalThis.PWNED=1;"`)
			req.Header.Set("CF-Visitor", `{"scheme":"https"}`)
		}
		mux.ServeHTTP(res, req)
		return res
	}

	poisoned := request("attacker.invalid", true)
	clean := request("honest.invalid", false)
	if poisoned.Code != http.StatusOK || clean.Code != http.StatusOK {
		t.Fatalf("unexpected index statuses %d and %d", poisoned.Code, clean.Code)
	}
	if !strings.HasPrefix(poisoned.Header().Get("Content-Type"), ctHTML) {
		t.Fatalf("unexpected index content type %q", poisoned.Header().Get("Content-Type"))
	}
	if poisoned.Body.String() != clean.Body.String() {
		t.Fatal("request headers changed the cached index")
	}
	if strings.Contains(clean.Body.String(), "attacker.invalid") || !strings.Contains(clean.Body.String(), config.CdnOrigin) {
		t.Fatal("index did not use the configured origin")
	}
}

func TestLegacyOriginIsConfigured(t *testing.T) {
	oldConfig := config
	config = DefaultConfig()
	config.CdnOrigin = "https://cdn.example.com"
	defer func() {
		config = oldConfig
	}()

	fs, err := storage.NewFSStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = fs.Put(
		"legacy/v135/@types/react@19.2.5/index.d.ts",
		strings.NewReader(`export * from "https://esm.sh/v135/react@19.2.5";`),
	); err != nil {
		t.Fatal(err)
	}
	if err = fs.Put(
		"legacy/v135/react@19.0.0.meta",
		strings.NewReader(`{"dts":"/v135/@types/react@latest/index.d.ts","code":"export default null"}`),
	); err != nil {
		t.Fatal(err)
	}

	mux := rex.New()
	mux.Use(esmLegacyRouter(fs))
	request := func(pathname string) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://attacker.invalid"+pathname, nil)
		req.Header.Set("X-Real-Origin", `https://esm.sh";globalThis.PWNED=1;"`)
		req.Header.Set("CF-Visitor", `{"scheme":"https"}`)
		mux.ServeHTTP(res, req)
		return res
	}

	dts := request("/v135/@types/react@19.2.5/index.d.ts")
	if dts.Code != http.StatusOK || strings.Contains(dts.Body.String(), "attacker.invalid") || !strings.Contains(dts.Body.String(), config.CdnOrigin+"/v135/") {
		t.Fatalf("unexpected legacy d.ts response %d: %s", dts.Code, dts.Body)
	}
	entry := request("/v135/react@19.0.0")
	if entry.Code != http.StatusOK || entry.Header().Get("X-TypeScript-Types") != config.CdnOrigin+"/v135/@types/react@latest/index.d.ts" {
		t.Fatalf("unexpected legacy entry response %d, types %q", entry.Code, entry.Header().Get("X-TypeScript-Types"))
	}
}

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

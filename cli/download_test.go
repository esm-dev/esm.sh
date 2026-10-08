package cli

import (
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/esm-dev/esm.sh/internal/importmap"
)

func TestDownloadFlags(t *testing.T) {
	const module = `export const version = "19.3.0";`
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("meta") {
			io.WriteString(w, `{"name":"react","version":"19.3.0"}`)
		} else if r.URL.Path == "/react@19.3.0/es2022/react.mjs" {
			w.Header().Set("Content-Type", "application/javascript")
			io.WriteString(w, module)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer cdn.Close()
	for _, flags := range [][]string{nil, {"--download"}, {"-D"}, {"-D", "--no-sri"}} {
		t.Run(strings.Join(flags, " "), func(t *testing.T) {
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, "src"), 0755); err != nil {
				t.Fatal(err)
			}
			source := fmt.Sprintf(`<head><script type="importmap">{"config":{"cdn":%q},"imports":{},"integrity":{"./vendor/react_19.3.0/es2022/react.mjs":"sha384-old"}}</script></head>`, cdn.URL)
			if err := os.WriteFile(filepath.Join(root, "index.html"), []byte(source), 0644); err != nil {
				t.Fatal(err)
			}
			args := append([]string{"-test.run=^TestCommandExitStatus$", "--", "add", "--no-prompt"}, flags...)
			cmd := exec.Command(os.Args[0], append(args, "react@19")...)
			cmd.Dir = filepath.Join(root, "src")
			cmd.Env = append(os.Environ(), "ESM_CLI_COMMAND_TEST=1")
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("add failed: %v\n%s", err, output)
			}
			data, err := os.ReadFile(filepath.Join(root, "index.html"))
			if err != nil {
				t.Fatal(err)
			}
			_, raw, _ := strings.Cut(string(data), `<script type="importmap">`)
			raw, _, _ = strings.Cut(raw, "</script>")
			var im importmap.ImportMapJson
			if err := json.Unmarshal([]byte(raw), &im); err != nil {
				t.Fatal(err)
			}
			if len(flags) == 0 {
				if im.Imports["react"] != cdn.URL+"/react@19.3.0/es2022/react.mjs" {
					t.Fatalf("default mapping changed: %v", im.Imports)
				}
				if _, err := os.Stat(filepath.Join(root, "vendor")); !os.IsNotExist(err) {
					t.Fatalf("default mode created vendor/: %v", err)
				}
				return
			}
			const entry = "./vendor/react_19.3.0/es2022/react.mjs"
			if im.Imports["react"] != entry {
				t.Fatalf("incorrect vendor entry: %v", im.Imports)
			}
			got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(entry)))
			if err != nil || string(got) != module {
				t.Fatalf("incorrect downloaded module: %s, %v", got, err)
			}
			if len(flags) == 2 {
				if len(im.Integrity) != 0 {
					t.Fatalf("--no-sri added integrity: %v", im.Integrity)
				}
			} else {
				hash := sha512.Sum384(got)
				if im.Integrity[entry] != "sha384-"+base64.StdEncoding.EncodeToString(hash[:]) {
					t.Fatalf("incorrect local integrity: %v", im.Integrity)
				}
			}
			if _, err := os.Stat(filepath.Join(root, "src", "vendor")); !os.IsNotExist(err) {
				t.Fatalf("vendor directory created below the project root: %v", err)
			}
		})
	}
}

func TestDownloadModuleGraph(t *testing.T) {
	var lock sync.Mutex
	requests := make(map[string]int)
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lock.Lock()
		requests[r.URL.RequestURI()]++
		lock.Unlock()
		w.Header().Set("Content-Type", "application/javascript")
		switch r.URL.Path {
		case "/app@1.0.0/es2022/app.mjs":
			io.WriteString(w, `import { peer } from "peer";
import "/node/process.mjs";
export { value } from "./chunk.mjs";
export const load = () => import("./lazy.mjs?mode=one");
export const loadTwo = () => import("./lazy.mjs?mode=two");
export const text = 'import "/not-an-import.mjs"';
export { redirect } from "./redirect.mjs";
export { default as data } from "./data.json" with { type: "json" };`)
		case "/app@1.0.0/es2022/chunk.mjs":
			io.WriteString(w, `import "./app.mjs"; export const value = 1;`)
		case "/app@1.0.0/es2022/lazy.mjs":
			fmt.Fprintf(w, `export default %q;`, r.URL.Query().Get("mode"))
		case "/app@1.0.0/es2022/data.json":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"value":1}`)
		case "/app@1.0.0/es2022/redirect.mjs":
			http.Redirect(w, r, "/other@1.0.0/es2022/entry.mjs", http.StatusFound)
		case "/other@1.0.0/es2022/entry.mjs":
			io.WriteString(w, `export { redirect } from "./deep.mjs";`)
		case "/other@1.0.0/es2022/deep.mjs":
			io.WriteString(w, `export const redirect = true;`)
		case "/@scope/peer@2.0.0/es2022/peer.mjs":
			io.WriteString(w, `export const peer = 2;`)
		case "/node/process.mjs":
			io.WriteString(w, `export default {};`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer cdn.Close()
	source := fmt.Sprintf(`{"config":{"cdn":%q},"imports":{"app":%q},"scopes":{%q:{"peer":%q}},"integrity":{%q:"sha384-old"}}`, cdn.URL, cdn.URL+"/app@1.0.0/es2022/app.mjs", cdn.URL+"/app@1.0.0/", cdn.URL+"/@scope/peer@2.0.0/es2022/peer.mjs", cdn.URL+"/app@1.0.0/es2022/app.mjs")
	im, err := importmap.Parse(nil, []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := downloadImports(im, root, false); err != nil {
		t.Fatal(err)
	}
	entry, _ := im.Imports.Get("app")
	if entry != "./vendor/app_1.0.0/es2022/app.mjs" {
		t.Fatalf("incorrect entry: %s", entry)
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(entry)))
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{`"../../@scope/peer_2.0.0/es2022/peer.mjs"`, `"../../node/process.mjs"`, `'import "/not-an-import.mjs"'`, `with { type: "json" }`} {
		if !strings.Contains(string(data), text) {
			t.Errorf("missing %s in rewritten module: %s", text, data)
		}
	}
	scope, ok := im.GetScopeImports("./vendor/app_1.0.0/")
	if !ok {
		t.Fatal("missing local scope")
	}
	if peer, _ := scope.Get("peer"); peer != "./vendor/@scope/peer_2.0.0/es2022/peer.mjs" {
		t.Fatalf("incorrect scoped dependency: %s", peer)
	}
	if _, ok := im.GetScopeImports(cdn.URL + "/app@1.0.0/"); ok {
		t.Fatal("remote scope was not relocated")
	}
	redirect, err := os.ReadFile(filepath.Join(root, "vendor/app_1.0.0/es2022/redirect.mjs"))
	if err != nil || !strings.Contains(string(redirect), `"../../other_1.0.0/es2022/deep.mjs"`) {
		t.Fatalf("redirect dependencies used the wrong base: %s, %v", redirect, err)
	}
	for _, localURL := range im.Integrity().Keys() {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(localURL)))
		if err != nil {
			t.Fatal(err)
		}
		hash := sha512.Sum384(body)
		if got, _ := im.Integrity().Get(localURL); got != "sha384-"+base64.StdEncoding.EncodeToString(hash[:]) {
			t.Errorf("incorrect integrity for %s", localURL)
		}
	}
	lazy, err := filepath.Glob(filepath.Join(root, "vendor/app_1.0.0/es2022/lazy_*.mjs"))
	if err != nil || len(lazy) != 2 {
		t.Fatalf("query variants were not downloaded separately: %v, %v", lazy, err)
	}
	lock.Lock()
	defer lock.Unlock()
	for path, count := range requests {
		if count != 1 || strings.Contains(path, "not-an-import") {
			t.Errorf("unexpected request count for %s: %d", path, count)
		}
	}
	if len(requests) != 10 {
		t.Fatalf("missing graph dependencies: %v", requests)
	}
}

func TestDownloadFailurePreservesFiles(t *testing.T) {
	t.Chdir(t.TempDir())
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("meta") {
			io.WriteString(w, `{"name":"react","version":"19.3.0"}`)
		} else if r.URL.Path == "/react@19.3.0/es2022/react.mjs" {
			io.WriteString(w, `import "./missing.mjs";`)
		} else {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		}
	}))
	defer cdn.Close()
	source := fmt.Sprintf(`<script type="importmap">{"config":{"cdn":%q},"imports":{"local":"./local.js"}}</script>`, cdn.URL)
	if err := os.WriteFile("index.html", []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	const filename = "vendor/react_19.3.0/es2022/react.mjs"
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte("existing module"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := updateImportMap([]string{"react@19"}, false, true, false, true); err == nil {
		t.Fatal("expected download failure")
	}
	for path, want := range map[string]string{"index.html": source, filename: "existing module"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("%s changed on failure: %q, %v", path, got, err)
		}
	}
	stages, err := filepath.Glob("vendor/.download-*")
	if err != nil || len(stages) != 0 {
		t.Fatalf("download staging directories were not removed: %v, %v", stages, err)
	}
}

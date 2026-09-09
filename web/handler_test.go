package web

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServeCSSModuleEscapesCSS(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "style.css"), []byte(`.icon:before{content:"\e001"}`), 0644); err != nil {
		t.Fatal(err)
	}
	handler := &Handler{config: &Config{AppDir: dir}}
	r := httptest.NewRequest(http.MethodGet, "/style.css?module", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	literal, _, ok := strings.Cut(strings.TrimPrefix(w.Body.String(), "const css="), ";let style,")
	var css string
	if !ok || json.Unmarshal([]byte(literal), &css) != nil {
		t.Fatalf("invalid CSS string literal: %s", literal)
	}
	if !strings.Contains(css, `\e001`) {
		t.Fatalf("CSS escape lost: %s", css)
	}
}

func TestFrameworkCSSUsesHTMLContent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tailwind.css"), []byte(`@import "tailwindcss";`), 0644); err != nil {
		t.Fatal(err)
	}
	var input bytes.Buffer
	handler := &Handler{
		config: &Config{AppDir: dir},
		loaderWorker: &JSWorker{
			stdin:     &input,
			outReader: bufio.NewReader(strings.NewReader(">>>css:\"first\"\n>>>css:\"second\"\n")),
		},
	}
	var etag string
	for i, class := range []string{"bg-red-500", "bg-blue-500"} {
		source := "<html><body><div class=\"" + class + "\"></div>" + strings.Repeat("<p class=\"p-4\"></p>", 1000) + "</body></html>"
		if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(source), 0644); err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodGet, "/tailwind.css", nil)
		r.Header.Set("If-None-Match", etag)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: status %d, body %s", i, w.Code, w.Body.String())
		}
		etag = w.Header().Get("Etag")
		var args []json.RawMessage
		if err := json.Unmarshal(input.Bytes(), &args); err != nil {
			t.Fatal(err)
		}
		var content string
		if err := json.Unmarshal(args[2], &content); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(content, "<div class=\""+class+"\">") {
			t.Fatalf("HTML content was overwritten: %.100s", content)
		}
		input.Reset()
	}
}

func TestAnalyzeDependencyTreeConcurrentLoads(t *testing.T) {
	dir := t.TempDir()
	var entry strings.Builder
	for i := range 64 {
		name := fmt.Sprintf("module%d.js", i)
		fmt.Fprintf(&entry, "import './%s';\n", name)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("console.log(1)"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	filename := filepath.Join(dir, "main.js")
	if err := os.WriteFile(filename, []byte(entry.String()), 0644); err != nil {
		t.Fatal(err)
	}
	handler := &Handler{config: &Config{AppDir: dir}}
	tree, err := handler.analyzeDependencyTree(filename, nil)
	if err != nil || len(tree) != 65 {
		t.Fatalf("tree contains %d files, error %v", len(tree), err)
	}
}

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/esm-dev/esm.sh/internal/importmap"
)

func TestUpdateImportMapFailurePreservesHTML(t *testing.T) {
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
			if err := updateImportMap([]string{"invalid package"}, false, true, true); err == nil {
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
	t.Chdir(t.TempDir())
	if err := os.WriteFile("index.html", []byte("<html><head></head></html>"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := updateImportMap(nil, false, true, true); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("index.html")
	if err != nil || !strings.Contains(string(got), "\"imports\": {}") || !strings.Contains(string(got), "</script>\n</head>") {
		t.Fatalf("invalid HTML: %q, %v", got, err)
	}
}

func TestAddImportsConcurrentErrors(t *testing.T) {
	specifiers := make([]string, 64)
	for i := range specifiers {
		specifiers[i] = "invalid package"
	}
	if addImports(importmap.Blank(), specifiers, false, true, true) {
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

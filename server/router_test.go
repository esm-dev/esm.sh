package server

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/ije/gox/utils"
)

func TestTreeShakeSavePath(t *testing.T) {
	savePath := "modules/react.mjs"
	a := treeShakeSavePath(savePath, []string{"default", "useState"})
	if a != treeShakeSavePath(savePath, []string{"default", "useState"}) {
		t.Fatal("expected deterministic tree-shake path")
	}
	if a == treeShakeSavePath(savePath, []string{"default", "useEffect"}) {
		t.Fatal("expected distinct export sets to use distinct paths")
	}
	digest := strings.TrimSuffix(strings.TrimPrefix(a, "modules/react_"), ".mjs")
	decoded, err := base64.RawURLEncoding.DecodeString(digest)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 32 {
		t.Fatalf("expected SHA-256 digest, got %d bytes", len(decoded))
	}
}

func TestCSSEntryRedirectURL(t *testing.T) {
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
			expected: "/@material/web@2.4.2-nightly.95dd57c.0/labs/gb/components/ripple/ripple.css",
		},
		{
			cssEntry: "labs/gb/components/ripple/ripple.css",
			expected: "/@material/web@2.4.2-nightly.95dd57c.0/labs/gb/components/ripple/ripple.css",
		},
	}

	for _, test := range tests {
		url := getCSSEntryRedirectURL(esmPath, test.cssEntry)
		if url != test.expected {
			t.Errorf("For CSSEntry %q, expected %q, but got %q", test.cssEntry, test.expected, url)
		}
	}
}

func TestRenderWorkerFactoryEscapesModulePath(t *testing.T) {
	modulePath := "/mod\" } = options; globalThis.PWNED = true; const { z = \"\n\u2028"
	code := renderWorkerFactory(modulePath)
	encodedPath := strings.TrimSpace(string(utils.MustEncodeJSON(modulePath)))

	if !strings.Contains(code, "new URL("+encodedPath+", import.meta.url)") {
		t.Fatalf("worker module path is not JSON-encoded: %s", code)
	}
	if strings.Contains(code, modulePath) || strings.ContainsRune(code, '\u2028') {
		t.Fatalf("worker module path is interpolated verbatim: %s", code)
	}
	if !strings.Contains(code, `JSON.stringify(moduleUrl)`) {
		t.Fatalf("worker import specifier is not encoded at runtime: %s", code)
	}
}

package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	esbuild "github.com/ije/esbuild-internal/api"
)

func TestSourceComplexity(t *testing.T) {
	for _, token := range []string{"(", "[", "{", "!", "~", "=", "?", ".", "<", "`", "new ", "do ", "declare ", "readonly ", "new\u00a0", `\u006eew `} {
		t.Run(token, func(t *testing.T) {
			if err := checkSourceComplexity(strings.Repeat(token, maxSourceComplexity+1)); err == nil {
				t.Fatal("expected source limit error")
			}
		})
	}
	for _, code := range []string{
		strings.Repeat("( /* ) */ ", maxSourceComplexity),
		strings.Repeat("( ')' , ", maxSourceComplexity),
		"export default `" + strings.Repeat("${(", maxSourceComplexity),
		"export default /" + strings.Repeat(`\(`, maxSourceComplexity) + "/;",
		"export default " + strings.Repeat("<X>", maxSourceComplexity),
	} {
		if err := checkSourceComplexity(code); err == nil {
			t.Fatal("expected source limit error")
		}
	}
	for _, code := range []string{
		strings.Repeat("(", maxSourceComplexity),
		strings.Repeat("new ", maxSourceComplexity),
		strings.Repeat("f();", maxSourceComplexity),
		"export default '" + strings.Repeat("x", 2*maxSourceComplexity) + "';",
		"/* " + strings.Repeat("text ", 2*maxSourceComplexity) + " */ export default 0;",
	} {
		if err := checkSourceComplexity(code); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSourceLimitPlugin(t *testing.T) {
	for _, ext := range []string{".js", ".jsx", ".ts", ".tsx", ".json", ".css", ".txt"} {
		t.Run(ext, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), "source"+ext)
			if err := os.WriteFile(filename, []byte(strings.Repeat("(", maxSourceComplexity+1)), 0644); err != nil {
				t.Fatal(err)
			}
			result := esbuild.Build(esbuild.BuildOptions{
				EntryPoints: []string{filename},
				Outdir:      t.TempDir(),
				Loader:      loaders,
				Plugins:     []esbuild.Plugin{sourceLimitPlugin},
			})
			if ext == ".txt" {
				if len(result.Errors) != 0 {
					t.Fatalf("text asset failed: %v", result.Errors)
				}
			} else if len(result.Errors) != 1 || result.Errors[0].Text != "Source is too complex" {
				t.Fatalf("expected source limit error, got %v", result.Errors)
			}
		})
	}
}

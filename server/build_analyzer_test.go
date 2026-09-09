package server

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/ije/gox/log"
)

func TestAnalyzeSplittingInvalidCache(t *testing.T) {
	for _, contents := range []string{"-1", "999999999999999999", "2\none.js", "0\none.js"} {
		t.Run(contents, func(t *testing.T) {
			wd := t.TempDir()
			pkgDir := filepath.Join(wd, "node_modules", "example")
			if err := os.MkdirAll(pkgDir, 0755); err != nil {
				t.Fatal(err)
			}
			for _, filename := range []string{"a.js", "b.js"} {
				if err := os.WriteFile(filepath.Join(pkgDir, filename), []byte("export const value = 1;"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			cachePath := filepath.Join(wd, "splitting.txt")
			if err := os.WriteFile(cachePath, []byte(contents), 0644); err != nil {
				t.Fatal(err)
			}
			logger, err := log.New("")
			if err != nil {
				t.Fatal(err)
			}
			logger.SetOutput(io.Discard)
			ctx := &BuildContext{
				wd: wd, target: "es2022", logger: logger,
				esmPath: EsmPath{PkgName: "example", PkgVersion: "1.0.0"},
				pkgJson: &npm.PackageJSON{
					Name: "example", Version: "1.0.0", Type: "module",
					Exports: npm.NewJSONObject([]string{"./a", "./b"}, map[string]any{"./a": "./a.js", "./b": "./b.js"}),
				},
			}
			ctx.analyzeSplitting()
			data, err := os.ReadFile(cachePath)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != "0" {
				t.Fatalf("invalid cache was not rebuilt: %q", data)
			}
		})
	}
}

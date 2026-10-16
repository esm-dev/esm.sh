package server

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
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
			cachePath := filepath.Join(wd, "splitting-v2-es2022.txt")
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

// errAfterContext reports cancellation after `n` calls to Err.
type errAfterContext struct {
	context.Context
	n atomic.Int32
}

func (c *errAfterContext) Err() error {
	if c.n.Add(-1) < 0 {
		return context.Canceled
	}
	return nil
}

func TestAnalyzeSplittingCacheKey(t *testing.T) {
	wd := t.TempDir()
	pkgDir := filepath.Join(wd, "node_modules", "example")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatal(err)
	}
	for name, code := range map[string]string{
		"a.js":      `export { value } from "./shared.js";`,
		"b.js":      `export { value } from "./shared.js";`,
		"shared.js": `export const value = 1;`,
	} {
		if err := os.WriteFile(filepath.Join(pkgDir, name), []byte(code), 0644); err != nil {
			t.Fatal(err)
		}
	}
	logger, err := log.New("")
	if err != nil {
		t.Fatal(err)
	}
	logger.SetOutput(io.Discard)
	newCtx := func(target string, dev bool, args BuildArgs) *BuildContext {
		return &BuildContext{
			wd: wd, target: target, dev: dev, args: args, logger: logger, npmrc: &NpmRC{},
			esmPath: EsmPath{PkgName: "example", PkgVersion: "1.0.0"},
			pkgJson: &npm.PackageJSON{
				Name: "example", Version: "1.0.0", Type: "module",
				Exports: npm.NewJSONObject([]string{"./a", "./b"}, map[string]any{"./a": "./a.js", "./b": "./b.js"}),
			},
		}
	}

	// a canceled analysis must not be cached
	canceled := newCtx("es2022", false, BuildArgs{})
	errCtx := &errAfterContext{Context: context.Background()}
	errCtx.n.Store(1)
	canceled.ctx = errCtx
	canceled.analyzeSplitting()
	if _, err := os.Stat(filepath.Join(wd, "splitting-v2-es2022.txt")); !os.IsNotExist(err) {
		t.Fatalf("canceled analysis was cached: %v", err)
	}

	args := BuildArgs{Conditions: []string{"custom"}}
	for _, ctx := range []*BuildContext{
		newCtx("es2022", false, BuildArgs{}),
		newCtx("node", false, BuildArgs{}),
		newCtx("es2022", true, BuildArgs{}),
		newCtx("es2022", false, args),
	} {
		ctx.analyzeSplitting()
	}
	files, err := filepath.Glob(filepath.Join(wd, "splitting-*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 4 {
		t.Fatalf("got splitting caches %v, want one per target, dev mode and args", files)
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "1\nshared.js" {
			t.Fatalf("%s = %q", filepath.Base(file), data)
		}
	}
}

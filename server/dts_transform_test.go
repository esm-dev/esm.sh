package server

import (
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/esm-dev/esm.sh/internal/storage"
)

func TestTransformDTS(t *testing.T) {
	t.Run("does not publish a parent when a dependency fails", func(t *testing.T) {
		root := t.TempDir()
		pkgDir := filepath.Join(root, "node_modules", "pkg")
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pkgDir, "index.d.ts"), []byte(`export * from "./child.d.ts";`), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pkgDir, "child.d.ts"), []byte(strings.Repeat("x", 1024*1024+1)), 0644); err != nil {
			t.Fatal(err)
		}
		fs, err := storage.NewFSStorage(filepath.Join(root, "storage"))
		if err != nil {
			t.Fatal(err)
		}
		ctx := &BuildContext{
			storage: fs,
			esmPath: EsmPath{PkgName: "pkg", PkgVersion: "1.0.0"},
			wd:      root,
			pkgJson: &npm.PackageJSON{},
		}
		if _, err := transformDTS(ctx, "./index.d.ts", "", nil); err == nil {
			t.Fatal("expected dependency transform to fail")
		}
		savePath := normalizeSavePath(path.Join("types", "/"+ctx.esmPath.PackageId(), "./index.d.ts"))
		if _, err := fs.Stat(savePath); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("parent was published after dependency failure: %v", err)
		}
	})

	t.Run("preserves remote specifiers and counts dependencies", func(t *testing.T) {
		root := t.TempDir()
		pkgDir := filepath.Join(root, "node_modules", "pkg")
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			t.Fatal(err)
		}
		const entry = `export * from "./child.d.ts";
export * from "https://example.com/types.d.ts";
export * from "npm:foo@1.2.3/sub";
`
		if err := os.WriteFile(filepath.Join(pkgDir, "index.d.ts"), []byte(entry), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pkgDir, "child.d.ts"), []byte("export interface Child {}"), 0644); err != nil {
			t.Fatal(err)
		}
		fs, err := storage.NewFSStorage(filepath.Join(root, "storage"))
		if err != nil {
			t.Fatal(err)
		}
		ctx := &BuildContext{
			storage:     fs,
			esmPath:     EsmPath{PkgName: "pkg", PkgVersion: "1.0.0"},
			wd:          root,
			pkgJson:     &npm.PackageJSON{},
			externalAll: true,
		}
		n, err := transformDTS(ctx, "./index.d.ts", "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("expected one related dts file, got %d", n)
		}

		savePath := normalizeSavePath(path.Join("types", "/"+ctx.esmPath.PackageId(), "./index.d.ts"))
		content, _, err := fs.Get(savePath)
		if err != nil {
			t.Fatal(err)
		}
		defer content.Close()
		data, err := io.ReadAll(content)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != entry {
			t.Fatalf("transformed dts not match, want:\n%s\ngot:\n%s", entry, data)
		}
	})
}

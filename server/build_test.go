package server

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/esm-dev/esm.sh/internal/storage"
)

func TestBuildModuleJSONPathTraversal(t *testing.T) {
	root := t.TempDir()
	wd := filepath.Join(root, "wd")
	pkgName := "traversal-pkg"
	pkgDir := filepath.Join(wd, "node_modules", pkgName)
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wd, "secret.json"), []byte(`{"secret":true}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "data.json"), []byte(`{"safe":true}`), 0644); err != nil {
		t.Fatal(err)
	}

	fs, err := storage.NewFSStorage(filepath.Join(root, "storage"))
	if err != nil {
		t.Fatal(err)
	}
	pkgJson := &npm.PackageJSON{Name: pkgName, Version: "1.0.0"}

	ctx := &BuildContext{
		storage: fs,
		wd:      wd,
		esmPath: EsmPath{
			PkgName:    pkgName,
			PkgVersion: "1.0.0",
			SubPath:    "../../secret.json",
		},
		pkgJson: pkgJson,
		path:    "traversal.mjs",
	}
	meta, _, err := ctx.buildModule(false)
	if err == nil {
		t.Fatal("expected path traversal to be rejected")
	}
	if meta != nil {
		t.Fatal("expected no build metadata")
	}
	if _, err := fs.Stat(ctx.getSavePath()); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected no cached module, got %v", err)
	}

	ctx = &BuildContext{
		storage: fs,
		wd:      wd,
		esmPath: EsmPath{
			PkgName:    pkgName,
			PkgVersion: "1.0.0",
			SubPath:    "data.json",
		},
		pkgJson: pkgJson,
		path:    "valid.mjs",
	}
	meta, _, err = ctx.buildModule(false)
	if err != nil {
		t.Fatal(err)
	}
	if meta == nil || !meta.ExportDefault {
		t.Fatal("expected a JSON module with a default export")
	}
	f, _, err := fs.Get(ctx.getSavePath())
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `export default {"safe":true}` {
		t.Fatalf("unexpected module output: %s", data)
	}
}

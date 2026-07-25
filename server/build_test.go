package server

import (
	"errors"
	"io"
	"os"
	"path"
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
	if err := os.WriteFile(filepath.Join(pkgDir, "invalid.json"), []byte(`{"safe":true};globalThis.PWNED=true`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "data.json"), []byte(`{"safe":"</script>"}`), 0644); err != nil {
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
			SubPath:    "invalid.json",
		},
		pkgJson: pkgJson,
		path:    "invalid.mjs",
	}
	meta, _, err = ctx.buildModule(false)
	if err == nil {
		t.Fatal("expected invalid JSON to be rejected")
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
	if string(data) != `export default {"safe":"\u003c/script\u003e"}` {
		t.Fatalf("unexpected module output: %s", data)
	}
}

func TestEncodeJSONModule(t *testing.T) {
	js, err := encodeJSONModule([]byte("{\"value\":\"</script>\u2028\"}"))
	if err != nil {
		t.Fatal(err)
	}
	if string(js) != `export default {"value":"\u003c/script\u003e\u2028"}` {
		t.Fatalf("unexpected module output: %s", js)
	}

	if _, err := encodeJSONModule([]byte(`{"safe":true};globalThis.PWNED=true`)); err == nil {
		t.Fatal("expected injected JavaScript to be rejected")
	}
}

func TestBuildStorageVersion(t *testing.T) {
	ctx := &BuildContext{path: "/package@1.0.0/es2022/package.mjs"}
	if got := ctx.getSavePath(); got != "modules/v2/package@1.0.0/es2022/package.mjs" {
		t.Fatalf("unexpected build storage path: %s", got)
	}
	if got := normalizeSavePath(path.Join(typesStoragePrefix, "/package@1.0.0/index.d.ts")); got != "types/v2/package@1.0.0/index.d.ts" {
		t.Fatalf("unexpected types storage path: %s", got)
	}
}

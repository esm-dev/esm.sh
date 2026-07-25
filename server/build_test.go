package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/esm-dev/esm.sh/internal/storage"
	"github.com/ije/gox/log"
)

func TestBuildJSONModule(t *testing.T) {
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

	if err := os.WriteFile(filepath.Join(pkgDir, "injected.json"), []byte(`{"safe":true};globalThis.PWNED=1`), 0644); err != nil {
		t.Fatal(err)
	}
	ctx = &BuildContext{
		storage: fs,
		wd:      wd,
		esmPath: EsmPath{
			PkgName:    pkgName,
			PkgVersion: "1.0.0",
			SubPath:    "injected.json",
		},
		pkgJson: pkgJson,
		path:    "injected.mjs",
	}
	meta, _, err = ctx.buildModule(false)
	if err == nil {
		t.Fatal("expected injected JSON to be rejected")
	}
	if meta != nil {
		t.Fatal("expected no build metadata")
	}
	if _, err := fs.Stat(ctx.getSavePath()); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected no injected module in storage, got %v", err)
	}

	ctx = &BuildContext{
		storage: fs,
		wd:      wd,
		esmPath: EsmPath{
			PkgName:    pkgName,
			PkgVersion: "1.0.0",
			SubPath:    "injected.json",
		},
		pkgJson: pkgJson,
		path:    "locked.mjs",
	}
	locked := []byte{}
	if err := fs.Put(ctx.getSavePath(), bytes.NewReader(locked)); err != nil {
		t.Fatal(err)
	}
	meta, _, err = ctx.buildModule(false)
	if err != nil {
		t.Fatal(err)
	}
	if meta == nil || !meta.ExportDefault {
		t.Fatal("expected build metadata for existing module")
	}
	f, _, err = fs.Get(ctx.getSavePath())
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	data, err = io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, locked) {
		t.Fatalf("existing module was overwritten: %s", data)
	}
}

func TestEncodeJSONModule(t *testing.T) {
	data := []byte("{\"html\":\"</script>\",\"line\":\"\u2028\u2029\"}")
	module, err := encodeJSONModule(data)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(module, []byte("</script>")) || bytes.Contains(module, []byte{0xe2, 0x80, 0xa8}) || bytes.Contains(module, []byte{0xe2, 0x80, 0xa9}) {
		t.Fatalf("unsafe JSON module: %s", module)
	}
	if _, err := encodeJSONModule([]byte(`{};globalThis.PWNED=1`)); err == nil {
		t.Fatal("expected invalid JSON to be rejected")
	}
}

func TestTransformDTSRepairsDependenciesWithoutOverwritingEntry(t *testing.T) {
	root := t.TempDir()
	wd := filepath.Join(root, "wd")
	pkgDir := filepath.Join(wd, "node_modules", "types-pkg")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "index.d.ts"), []byte(`export * from "./dep";`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "dep.d.ts"), []byte(`export type Value = string;`), 0644); err != nil {
		t.Fatal(err)
	}
	fs, err := storage.NewFSStorage(filepath.Join(root, "storage"))
	if err != nil {
		t.Fatal(err)
	}
	const entryPath = "types/types-pkg@1.0.0/index.d.ts"
	ctx := &BuildContext{
		storage: fs,
		wd:      wd,
		esmPath: EsmPath{PkgName: "types-pkg", PkgVersion: "1.0.0"},
	}
	if _, err = transformDTS(ctx, "./index.d.ts", "", nil); err != nil {
		t.Fatal(err)
	}
	entry, _, err := fs.Get(entryPath)
	if err != nil {
		t.Fatal(err)
	}
	locked, err := io.ReadAll(entry)
	entry.Close()
	if err != nil {
		t.Fatal(err)
	}
	const depPath = "types/types-pkg@1.0.0/dep.d.ts"
	if err = fs.Delete(depPath); err != nil {
		t.Fatal(err)
	}
	if _, err = transformDTS(ctx, "./index.d.ts", "", nil); err != nil {
		t.Fatal(err)
	}
	entry, _, err = fs.Get(entryPath)
	if err != nil {
		t.Fatal(err)
	}
	entryData, err := io.ReadAll(entry)
	entry.Close()
	if err != nil || !bytes.Equal(entryData, locked) {
		t.Fatalf("stored entry changed: %q, err %v", entryData, err)
	}
	dep, _, err := fs.Get(depPath)
	if err != nil {
		t.Fatal(err)
	}
	depData, err := io.ReadAll(dep)
	dep.Close()
	if err != nil || !bytes.Contains(depData, []byte("Value")) {
		t.Fatalf("dependency was not repaired: %q, err %v", depData, err)
	}
}

func TestPutImmutableKeepsExistingObject(t *testing.T) {
	fs, err := storage.NewFSStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = fs.Put("module.mjs", strings.NewReader("first")); err != nil {
		t.Fatal(err)
	}
	stored, err := putImmutable(fs, "module.mjs", []byte("second"))
	if err != nil || string(stored) != "first" {
		t.Fatalf("unexpected immutable result %q, err %v", stored, err)
	}
	if err = putImmutableExact(fs, "module.mjs", []byte("second")); err == nil {
		t.Fatal("expected immutable conflict")
	}
}

func TestBuildRejectsImmutableModuleConflict(t *testing.T) {
	root := t.TempDir()
	wd := filepath.Join(root, "wd")
	pkgDir := filepath.Join(wd, "node_modules", "locked-pkg")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "index.js"), []byte(`export const value = 1;`), 0644); err != nil {
		t.Fatal(err)
	}
	fs, err := storage.NewFSStorage(filepath.Join(root, "storage"))
	if err != nil {
		t.Fatal(err)
	}
	logger, err := log.New("")
	if err != nil {
		t.Fatal(err)
	}
	ctx := &BuildContext{
		logger:  logger,
		metaDB:  NewBuildMetaDB(fs),
		storage: fs,
		wd:      wd,
		esmPath: EsmPath{
			GhPrefix:   true,
			PkgName:    "locked-pkg",
			PkgVersion: "main",
		},
		pkgJson: &npm.PackageJSON{
			Name:    "locked-pkg",
			Version: "main",
			Type:    "module",
			Module:  "./index.js",
		},
		target: "es2022",
		path:   "/locked-pkg@main/es2022/locked-pkg.mjs",
	}
	savePath := ctx.getSavePath()
	if err = fs.Put(savePath, strings.NewReader("locked")); err != nil {
		t.Fatal(err)
	}
	if _, err = ctx.Build(context.Background()); err == nil || !strings.Contains(err.Error(), "immutable storage conflict") {
		t.Fatalf("expected immutable conflict, got %v", err)
	}
	f, _, err := fs.Get(savePath)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := io.ReadAll(f)
	f.Close()
	if err != nil || string(stored) != "locked" {
		t.Fatalf("stored module changed: %q, err %v", stored, err)
	}
	if _, err = ctx.metaDB.Get(ctx.Path()); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("conflicting metadata was published: %v", err)
	}
}

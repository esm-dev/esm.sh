package server

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/esm-dev/esm.sh/internal/storage"
	"github.com/ije/gox/log"
)

func TestBuildModuleNativeEntry(t *testing.T) {
	oldConfig, oldClient := config, http.DefaultClient
	config = &Config{WorkDir: t.TempDir()}
	http.DefaultClient = &http.Client{Transport: ghTestTransport(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("native entry must not install the CommonJS lexer")
	})}
	t.Cleanup(func() { config, http.DefaultClient = oldConfig, oldClient })

	const name = "@oxfmt/binding-darwin-arm64"
	const main = "./oxfmt.darwin-arm64.node"
	for _, pkgType := range []string{"commonjs", "module"} {
		for _, subPath := range []string{"", "binding"} {
			t.Run(pkgType+"/"+subPath, func(t *testing.T) {
				wd := t.TempDir()
				pkgDir := filepath.Join(wd, "node_modules", name)
				if err := os.MkdirAll(pkgDir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(pkgDir, main), []byte{0xcf, 0xfa, 0xed, 0xfe}, 0644); err != nil {
					t.Fatal(err)
				}
				logger, err := log.New("")
				if err != nil {
					t.Fatal(err)
				}
				logger.SetOutput(io.Discard)
				ctx := &BuildContext{
					wd: wd, target: "es2022", logger: logger,
					esmPath: EsmPath{PkgName: name, PkgVersion: "0.40.0", SubPath: subPath},
					pkgJson: &npm.PackageJSON{
						Name: name, Version: "0.40.0", Type: pkgType, Main: main,
						Exports: npm.NewJSONObject([]string{"./binding"}, map[string]any{"./binding": main}),
					},
				}
				meta, _, err := ctx.buildModule(false)
				if err == nil || err.Error() != `unsupported node native module "./oxfmt.darwin-arm64.node"` {
					t.Fatalf("expected native module error, got: %v", err)
				}
				if meta != nil {
					t.Fatal("expected no build metadata")
				}
			})
		}
	}
}

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

func TestBuildTypesArgs(t *testing.T) {
	for _, filename := range []string{"index.d.ts", "index.d.mts", "index.d.cts"} {
		t.Run(filename, func(t *testing.T) {
			wd := t.TempDir()
			pkgDir := filepath.Join(wd, "node_modules", "example")
			if err := os.MkdirAll(pkgDir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(pkgDir, filename), []byte("export {};"), 0644); err != nil {
				t.Fatal(err)
			}
			fs, err := storage.NewFSStorage(filepath.Join(wd, "storage"))
			if err != nil {
				t.Fatal(err)
			}
			ctx := &BuildContext{
				wd: wd, target: "types", storage: fs,
				esmPath: EsmPath{PkgName: "example", PkgVersion: "1.0.0", SubPath: filename},
				pkgJson: &npm.PackageJSON{Name: "example", Version: "1.0.0", Types: "./" + filename},
				args:    BuildArgs{Conditions: []string{"custom"}},
			}
			want := "/example@1.0.0/" + ctx.getBuildArgsPrefix(true) + filename
			if got := ctx.Path(); got != want {
				t.Errorf("build path = %q, want %q", got, want)
			}
			meta, err := ctx.buildTypes()
			if err != nil {
				t.Fatal(err)
			}
			if meta.Dts != want {
				t.Errorf("types metadata = %q, want %q", meta.Dts, want)
			}
			if _, err := fs.Stat(normalizeSavePath("types" + want)); err != nil {
				t.Fatalf("missing transformed types: %v", err)
			}
			ctx.target = "es2022"
			ctx.path = ""
			meta, _, err = ctx.buildModule(false)
			if err != nil {
				t.Fatal(err)
			}
			if meta.Dts != want {
				t.Errorf("types-only module metadata = %q, want %q", meta.Dts, want)
			}
		})
	}
}

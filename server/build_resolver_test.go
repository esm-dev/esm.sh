package server

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/esm-dev/esm.sh/internal/storage"
	esbuild "github.com/ije/esbuild-internal/api"
	"github.com/ije/gox/log"
	"github.com/ije/gox/set"
)

func TestResolveExternalModuleSideEffects(t *testing.T) {
	for _, specifier := range []string{"example", "example/init"} {
		for _, tt := range []struct {
			name    string
			listed  []string
			pure    bool
			missing bool
			keep    bool
		}{
			{name: "listed", listed: []string{"./init.js"}, keep: true},
			{name: "listed without dot", listed: []string{"init.js"}, keep: true},
			{name: "unlisted", listed: []string{"./other.js"}},
			{name: "unspecified", keep: true},
			{name: "false", pure: true},
			{name: "unresolved", listed: []string{"./other.js"}, missing: true, keep: true},
		} {
			t.Run(specifier+"/"+tt.name, func(t *testing.T) {
				wd := t.TempDir()
				pkgDir := filepath.Join(wd, "node_modules", "example")
				if err := os.MkdirAll(pkgDir, 0755); err != nil {
					t.Fatal(err)
				}
				if !tt.missing {
					if err := os.WriteFile(filepath.Join(pkgDir, "init.js"), []byte("globalThis.initialized = true; export {};"), 0644); err != nil {
						t.Fatal(err)
					}
				}
				ctx := &BuildContext{
					wd: wd, target: "es2022", esmPath: EsmPath{PkgName: "example", PkgVersion: "1.0.0"},
					pkgJson: &npm.PackageJSON{
						Name: "example", Version: "1.0.0", Type: "module", Main: "./init.js",
						SideEffects: *set.NewReadOnly(tt.listed...), SideEffectsFalse: tt.pure,
					},
				}
				resolved, sideEffects, err := ctx.resolveExternalModule(specifier, esbuild.ResolveJSImportStatement, false, false)
				if err != nil {
					t.Fatal(err)
				}
				result := esbuild.Build(esbuild.BuildOptions{
					Stdin:  &esbuild.StdinOptions{Contents: fmt.Sprintf("import %q; export const value = 1;", specifier)},
					Bundle: true, Format: esbuild.FormatESModule,
					Plugins: []esbuild.Plugin{{Name: "external", Setup: func(build esbuild.PluginBuild) {
						build.OnResolve(esbuild.OnResolveOptions{Filter: ".*"}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
							return esbuild.OnResolveResult{Path: resolved, External: true, SideEffects: sideEffects}, nil
						})
					}}},
				})
				if len(result.Errors) != 0 {
					t.Fatal(result.Errors)
				}
				output := string(result.OutputFiles[0].Contents)
				if strings.Contains(output, resolved) != tt.keep {
					t.Fatalf("import retained = %v, want %v:\n%s", !tt.keep, tt.keep, output)
				}
			})
		}
	}
}

func TestResolveExternalModuleScopedFork(t *testing.T) {
	for _, dependency := range []string{"", "dependencies", "peerDependencies"} {
		for _, subpath := range []string{"", "/tsl"} {
			t.Run(dependency+subpath, func(t *testing.T) {
				pkg := &npm.PackageJSON{Name: "@scope/three", Version: "1.0.0"}
				want := "/@scope/three@1.0.0/es2022/"
				switch dependency {
				case "dependencies":
					pkg.Dependencies = map[string]string{"three": "2.0.0"}
					want = "/three@2.0.0/es2022/"
				case "peerDependencies":
					pkg.PeerDependencies = map[string]string{"three": "2.0.0"}
					want = "/three@2.0.0/es2022/"
				}
				if subpath == "" {
					want += "three.mjs"
				} else {
					want += "tsl.mjs"
				}
				ctx := &BuildContext{target: "es2022", esmPath: EsmPath{PkgName: pkg.Name, PkgVersion: pkg.Version}, pkgJson: pkg}
				got, _, err := ctx.resolveExternalModule("three"+subpath, esbuild.ResolveJSImportStatement, false, false)
				if err != nil || got != want {
					t.Fatalf("resolved = %q, %v; want %q", got, err, want)
				}
			})
		}
	}
}

func TestBuildModuleScopedForkSubpath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("esbuild plugin path resolution differs on windows")
	}
	wd := t.TempDir()
	pkgDir := filepath.Join(wd, "node_modules", "@scope", "three")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "addon.js"), []byte(`export { value } from "three/tsl";`), 0644); err != nil {
		t.Fatal(err)
	}
	fs, err := storage.NewFSStorage(filepath.Join(wd, "storage"))
	if err != nil {
		t.Fatal(err)
	}
	logger, err := log.New("")
	if err != nil {
		t.Fatal(err)
	}
	logger.SetOutput(io.Discard)
	ctx := &BuildContext{
		wd: wd, target: "es2022", storage: fs, logger: logger, npmrc: &NpmRC{},
		esmPath: EsmPath{PkgName: "@scope/three", PkgVersion: "1.0.0", SubPath: "addon"},
		pkgJson: &npm.PackageJSON{Name: "@scope/three", Version: "1.0.0", Type: "module", Types: "./index.d.ts"},
	}
	meta, _, err := ctx.buildModule(false)
	if err != nil {
		t.Fatal(err)
	}
	f, _, err := fs.Get(ctx.getSavePath())
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	output, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	const want = "/@scope/three@1.0.0/es2022/tsl.mjs"
	if len(meta.Imports) != 1 || meta.Imports[0] != want || !strings.Contains(string(output), want) {
		t.Fatalf("expected subpath import, got %v:\n%s", meta.Imports, output)
	}
}

func TestResolveEntryWildcardSuffix(t *testing.T) {
	wd := t.TempDir()
	pkgDir := filepath.Join(wd, "node_modules", "example", "src")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, filename := range []string{"one.js", "one.d.ts"} {
		if err := os.WriteFile(filepath.Join(pkgDir, filename), []byte("export {};"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, conditions := range []any{
		"./src/*.js",
		npm.NewJSONObject([]string{"types", "import"}, map[string]any{"types": "./src/*.d.ts", "import": "./src/*.js"}),
	} {
		esm := EsmPath{PkgName: "example", PkgVersion: "1.0.0", SubPath: "features/one-feature"}
		ctx := &BuildContext{
			wd: wd, target: "es2022", esmPath: esm,
			pkgJson: &npm.PackageJSON{
				Name: "example", Version: "1.0.0", Type: "module",
				Exports: npm.NewJSONObject([]string{"./features/*-feature"}, map[string]any{"./features/*-feature": conditions}),
			},
		}
		if entry := ctx.resolveEntry(esm); entry.main != "./src/one.js" || entry.types != "./src/one.d.ts" || !entry.module {
			t.Fatalf("unexpected entry: %+v", entry)
		}
	}
	for _, tt := range []struct {
		pattern, subpath, want string
		match                  bool
	}{
		{"./features/*", "features/one", "one", true},
		{"./features/*-feature", "features/one-feature", "one", true},
		{"./features/*-feature", "features/nested/one-feature", "nested/one", true},
		{"./features/*-feature", "features/one", "", false},
		{"./features/*-feature", "other/one-feature", "", false},
		{"./features/*features", "features", "", false},
	} {
		if got, ok := matchAsteriskExport(tt.pattern, tt.subpath); got != tt.want || ok != tt.match {
			t.Errorf("match(%q, %q) = %q, %v; want %q, %v", tt.pattern, tt.subpath, got, ok, tt.want, tt.match)
		}
	}
}

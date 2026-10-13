package server

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	for _, tt := range []struct{ pattern, subpath string }{
		{"./features/*-feature", "features/one-feature"},
		{"./*.js", "one.js"},
		{"./features/*.js", "features/one.js"},
		{"./features/*.mjs", "features/one.mjs"},
		{"./features/*.cjs", "features/one.cjs"},
		{"./features/*-feature.js", "features/one-feature.js"},
	} {
		for i, conditions := range []any{
			"./src/*.js",
			npm.NewJSONObject([]string{"types", "import"}, map[string]any{"types": "./src/*.d.ts", "import": "./src/*.js"}),
		} {
			t.Run(fmt.Sprintf("%s/%d", tt.pattern, i), func(t *testing.T) {
				esm, _, _, _, _, err := parseEsmPathSyntax("/example@1.0.0/" + tt.subpath)
				if err != nil {
					t.Fatal(err)
				}
				ctx := &BuildContext{
					wd: wd, target: "es2022", esmPath: esm,
					pkgJson: &npm.PackageJSON{
						Name: "example", Version: "1.0.0", Type: "module",
						Exports: npm.NewJSONObject([]string{tt.pattern}, map[string]any{tt.pattern: conditions}),
					},
				}
				if entry := ctx.resolveEntry(esm); entry.main != "./src/one.js" || entry.types != "./src/one.d.ts" || !entry.module {
					t.Fatalf("unexpected entry: %+v", entry)
				}
			})
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

func TestResolveExternalAllSelfModule(t *testing.T) {
	for _, tt := range []struct {
		name, specifier, external, want string
		noExports                       bool
	}{
		{name: "private", specifier: "@scope/example/dist/shared-hash", want: "./dist/shared-hash.mjs"},
		{name: "explicit package", specifier: "@scope/example/dist/shared-hash", external: "@scope/example", want: "@scope/example/dist/shared-hash"},
		{name: "explicit namespace", specifier: "@scope/example/dist/shared-hash", external: "@scope", want: "@scope/example/dist/shared-hash"},
		{name: "main", specifier: "@scope/example", want: "@scope/example"},
		{name: "public", specifier: "@scope/example/public", want: "@scope/example/public"},
		{name: "wildcard", specifier: "@scope/example/features/one", want: "@scope/example/features/one"},
		{name: "no exports", specifier: "@scope/example/dist/shared-hash", noExports: true, want: "@scope/example/dist/shared-hash"},
		{name: "dependency", specifier: "dependency/private", want: "dependency/private"},
		{name: "builtin", specifier: "node:fs", want: "node:fs"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pkg := &npm.PackageJSON{
				Name: "@scope/example", Version: "1.0.0", Type: "module",
				Exports: npm.NewJSONObject([]string{".", "./public", "./features/*"}, map[string]any{
					".": "./index.js", "./public": "./public.js", "./features/*": "./features/*.js",
				}),
			}
			if tt.noExports {
				pkg.Exports = npm.JSONObject{}
			}
			ctx := &BuildContext{
				target: "es2022", externalAll: true, pkgJson: pkg,
				esmPath: EsmPath{PkgName: pkg.Name, PkgVersion: pkg.Version},
			}
			if tt.external != "" {
				ctx.args.External = *set.NewReadOnly(tt.external)
			}
			got, _, err := ctx.resolveExternalModule(tt.specifier, esbuild.ResolveJSImportStatement, false, false)
			if err != nil || got != tt.want {
				t.Fatalf("resolved = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestBuildModuleExternalAllSplitting(t *testing.T) {
	wd := t.TempDir()
	pkgDir := filepath.Join(wd, "node_modules", "@scope", "example", "dist")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatal(err)
	}
	for filename, contents := range map[string]string{
		"index.js":       `export { state } from "./shared-hash.js";`,
		"other.js":       `export { state } from "./shared-hash.js";`,
		"shared-hash.js": `import { value } from "dependency"; export const state = { value };`,
	} {
		if err := os.WriteFile(filepath.Join(pkgDir, filename), []byte(contents), 0644); err != nil {
			t.Fatal(err)
		}
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
	pkg := &npm.PackageJSON{
		Name: "@scope/example", Version: "1.0.0", Type: "module", Types: "./index.d.ts",
		Exports:      npm.NewJSONObject([]string{".", "./other"}, map[string]any{".": "./dist/index.js", "./other": "./dist/other.js"}),
		Dependencies: map[string]string{"dependency": "1.0.0"},
	}
	for _, subPath := range []string{"", "other", "dist/shared-hash"} {
		t.Run(subPath, func(t *testing.T) {
			ctx := &BuildContext{
				wd: wd, target: "es2022", externalAll: true, storage: fs, logger: logger, npmrc: &NpmRC{}, pkgJson: pkg,
				esmPath: EsmPath{PkgName: pkg.Name, PkgVersion: pkg.Version, SubPath: subPath},
			}
			ctx.analyzeSplitting()
			if ctx.splitting == nil || !ctx.splitting.Has("dist/shared-hash.js") {
				t.Fatal("shared module was not split")
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
			want := `"./dist/shared-hash.mjs"`
			if subPath == "dist/shared-hash" {
				want = `"dependency"`
				if len(meta.Imports) != 0 {
					t.Fatalf("dependency must remain external, got %v", meta.Imports)
				}
			} else if len(meta.Imports) != 1 || meta.Imports[0] != "/*@scope/example@1.0.0/es2022/dist/shared-hash.mjs" {
				t.Errorf("unexpected shared module imports: %v", meta.Imports)
			}
			if !strings.Contains(string(output), want) {
				t.Fatalf("missing %s:\n%s", want, output)
			}
		})
	}
}

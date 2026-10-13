package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/esm-dev/esm.sh/internal/storage"
	esbuild "github.com/ije/esbuild-internal/api"
	"github.com/ije/gox/log"
	"github.com/ije/gox/set"
)

func TestBuildModuleBundleNodeRuntime(t *testing.T) {
	oldConfig := config
	config = &Config{SourceMap: true}
	t.Cleanup(func() { config = oldConfig })
	for _, tt := range []struct {
		name        string
		code        string
		target      string
		mode        BundleMode
		external    []string
		externalAll bool
		imports     []string
	}{
		{name: "globals", code: `export const env = process.env; export const bytes = Buffer.from("hello");`, mode: BundleDeps},
		{name: "builtins", code: `export { default } from "node:process"; export { Buffer } from "node:buffer"; export { readFile } from "node:fs/promises"; export { inspect } from "sys"; export const value = 1;`, mode: BundleDeps},
		{name: "require", code: `export const buffer = require("node:buffer");`, mode: BundleDeps},
		{name: "dynamic import", code: `export const load = () => import("node:buffer");`, mode: BundleDeps},
		{name: "no polyfills", code: `export const env = process.env.NODE_ENV;`, mode: BundleDeps},
		{name: "default", code: `export const env = process.env;`, imports: []string{"/node/process.mjs"}},
		{name: "no bundle", code: `export const env = process.env;`, mode: BundleFalse, imports: []string{"/node/process.mjs"}},
		{name: "external", code: `export const env = process.env; export { Buffer } from "node:buffer";`, mode: BundleDeps, external: []string{"node:process", "node:buffer"}, imports: []string{"node:buffer", "node:process"}},
		{name: "mixed external", code: `export const env = process.env; export { Buffer } from "node:buffer";`, mode: BundleDeps, external: []string{"node:buffer"}, imports: []string{"node:buffer"}},
		{name: "external all", code: `export const env = process.env; export { Buffer } from "node:buffer";`, mode: BundleDeps, externalAll: true, imports: []string{"/node/process.mjs", "node:buffer"}},
		{name: "peer", code: `export const env = process.env; export { value } from "peer";`, mode: BundleDeps, imports: []string{"/peer@1.0.0/es2022/peer.mjs"}},
		{name: "node", code: `export { default } from "node:process";`, mode: BundleDeps, target: "node", imports: []string{"node:process"}},
		{name: "denonext", code: `export const env = process.env;`, mode: BundleDeps, target: "denonext", imports: []string{"node:process"}},
	} {
		for _, minify := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/minify=%v", tt.name, minify), func(t *testing.T) {
				config.Minify = minify
				wd := t.TempDir()
				pkgDir := filepath.Join(wd, "node_modules", "example")
				if err := os.MkdirAll(pkgDir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(pkgDir, "index.js"), []byte(tt.code), 0644); err != nil {
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
				target := tt.target
				if target == "" {
					target = "es2022"
				}
				ctx := &BuildContext{
					wd: wd, target: target, bundleMode: tt.mode, externalAll: tt.externalAll, storage: fs, logger: logger, npmrc: &NpmRC{},
					esmPath: EsmPath{PkgName: "example", PkgVersion: "1.0.0"},
					args:    BuildArgs{External: *set.NewReadOnly(tt.external...)},
					pkgJson: &npm.PackageJSON{Name: "example", Version: "1.0.0", Type: "module", Main: "./index.js", Types: "./index.d.ts", PeerDependencies: map[string]string{"peer": "1.0.0"}},
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
				var imports []string
				result := esbuild.Build(esbuild.BuildOptions{
					Stdin: &esbuild.StdinOptions{Contents: string(output)}, Bundle: true, Format: esbuild.FormatESModule,
					Plugins: []esbuild.Plugin{{Name: "imports", Setup: func(build esbuild.PluginBuild) {
						build.OnResolve(esbuild.OnResolveOptions{Filter: ".*"}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
							imports = append(imports, args.Path)
							return esbuild.OnResolveResult{Path: args.Path, External: true}, nil
						})
					}}},
				})
				if len(result.Errors) > 0 {
					t.Fatal(result.Errors)
				}
				slices.Sort(imports)
				if !slices.Equal(imports, tt.imports) {
					t.Errorf("imports = %v, want %v", imports, tt.imports)
				}
				var wantMetaImports []string
				for _, specifier := range tt.imports {
					if strings.HasPrefix(specifier, "/") {
						wantMetaImports = append(wantMetaImports, specifier)
					}
				}
				if !slices.Equal(meta.Imports, wantMetaImports) {
					t.Errorf("metadata imports = %v, want %v", meta.Imports, wantMetaImports)
				}
				f, _, err = fs.Get(ctx.getSavePath() + ".map")
				if err != nil {
					t.Fatal(err)
				}
				defer f.Close()
				var sourceMap struct {
					SourcesContent []string `json:"sourcesContent"`
				}
				if err := json.NewDecoder(f).Decode(&sourceMap); err != nil {
					t.Fatal(err)
				}
				if !slices.Contains(sourceMap.SourcesContent, tt.code) {
					t.Error("source map lost the original module")
				}
			})
		}
	}
}

func TestBuildModuleConcurrentResolvers(t *testing.T) {
	wd := t.TempDir()
	pkgDir := filepath.Join(wd, "node_modules", "example")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatal(err)
	}
	const count = 100
	var entry strings.Builder
	deps := map[string]string{}
	for i := range count {
		deps[fmt.Sprintf("dep-%d", i)] = "1.0.0"
		deps[fmt.Sprintf("esm-%d", i)] = "1.0.0"
		fmt.Fprintf(&entry, "export { value as value%d } from './mod-%d.js';\n", i, i)
		files := map[string]string{
			fmt.Sprintf("mod-%d.js", i): fmt.Sprintf(`import { value as imported } from "esm-%d";
import { leaf } from "./leaf-%d.js";
export const value = require("dep-%d").default + imported + leaf + require("object-assign")({}, { value: %d }).value;`, i, i, i, i),
			fmt.Sprintf("leaf-%d.js", i): fmt.Sprintf("export const leaf = %d;", i),
		}
		for name, content := range files {
			if err := os.WriteFile(filepath.Join(pkgDir, name), []byte(content), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "index.js"), []byte(entry.String()), 0644); err != nil {
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
	for _, analyze := range []bool{false, true} {
		t.Run(fmt.Sprintf("analyze=%v", analyze), func(t *testing.T) {
			ctx := &BuildContext{
				wd: wd, target: "es2022", storage: fs, logger: logger, npmrc: &NpmRC{},
				esmPath: EsmPath{PkgName: "example", PkgVersion: "1.0.0"},
				pkgJson: &npm.PackageJSON{Name: "example", Version: "1.0.0", Type: "module", Main: "./index.js", Types: "./index.d.ts", Dependencies: deps},
			}
			ctx.Path()
			meta, includes, err := ctx.buildModule(analyze)
			if err != nil {
				t.Fatal(err)
			}
			if analyze {
				if len(includes) != count*2 {
					t.Fatalf("got %d dependency edges, want %d", len(includes), count*2)
				}
				for i := range count {
					for _, edge := range [][2]string{
						{fmt.Sprintf("mod-%d.js", i), "index.js"},
						{fmt.Sprintf("leaf-%d.js", i), fmt.Sprintf("mod-%d.js", i)},
					} {
						if !slices.Contains(includes, edge) {
							t.Errorf("missing dependency edge %v", edge)
						}
					}
				}
				return
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
			for i := range count {
				if !strings.Contains(string(output), fmt.Sprintf(`case"dep-%d":`, i)) {
					t.Errorf("missing require case for dep-%d", i)
				}
				for _, name := range []string{fmt.Sprintf("dep-%d", i), fmt.Sprintf("esm-%d", i)} {
					if !slices.Contains(meta.Imports, "/"+name+"@1.0.0/es2022/"+name+".mjs") {
						t.Errorf("missing import %q", name)
					}
				}
			}
			if !strings.Contains(string(output), `case"npm:object-assign":`) {
				t.Error("missing require case for replacement module")
			}
		})
	}
}

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

func TestBuildModulePrivateNodeModules(t *testing.T) {
	wd := t.TempDir()
	pkgDir := filepath.Join(wd, "node_modules")
	for name, content := range map[string]string{
		"example/src/index.js":                      `export { value } from "lib/value"; export const commonjs = require("lib/value").default;`,
		"example/src/node_modules/lib/value.js":     `import { number } from "types/numbers"; import { dep } from "dep"; export const value = number + dep; export default value;`,
		"example/src/node_modules/types/numbers.js": `export { number } from "number";`,
		"example/src/node_modules/number.js":        `export const number = 42;`,
		"dep/package.json":                          `{ "name": "dep", "version": "1.0.0", "type": "module" }`,
		"dep/index.js":                              `export const dep = 1;`,
	} {
		filename := filepath.Join(pkgDir, name)
		if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
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
	for _, target := range []string{"es2022", "denonext"} {
		t.Run(target, func(t *testing.T) {
			ctx := &BuildContext{
				wd: wd, target: target, storage: fs, logger: logger, npmrc: &NpmRC{},
				esmPath: EsmPath{PkgName: "example", PkgVersion: "1.0.0"},
				pkgJson: &npm.PackageJSON{Name: "example", Version: "1.0.0", Type: "module", Main: "./src/index.js", Types: "./index.d.ts", Dependencies: map[string]string{"dep": "1.0.0"}},
			}
			meta, _, err := ctx.buildModule(false)
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"/dep@1.0.0/" + target + "/dep.mjs"}
			if !slices.Equal(meta.Imports, want) {
				t.Errorf("imports = %v, want %v", meta.Imports, want)
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
			if !strings.Contains(string(output), "42") {
				t.Error("missing private module value")
			}
		})
	}
}

func TestBuildModulePrivateNodeModulesExternal(t *testing.T) {
	wd := t.TempDir()
	for name, content := range map[string]string{
		"index.js":                         `export { value } from "lib/value"; export { value as scoped } from "@scope/lib/value";`,
		"node_modules/lib/value.js":        `export const value = 42;`,
		"node_modules/@scope/lib/value.js": `export const value = 99;`,
	} {
		filename := filepath.Join(wd, "node_modules", "example", name)
		if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
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
	for _, tt := range []struct {
		name     string
		external []string
		all      bool
		mode     BundleMode
		imports  []string
	}{
		{name: "default"},
		{name: "package", external: []string{"lib"}, imports: []string{"lib/value"}},
		{name: "package with bundle", mode: BundleDeps, external: []string{"lib"}, imports: []string{"lib/value"}},
		{name: "scoped package", external: []string{"@scope/lib"}, imports: []string{"@scope/lib/value"}},
		{name: "namespace", external: []string{"@scope"}, imports: []string{"@scope/lib/value"}},
		{name: "namespace with bundle", mode: BundleDeps, external: []string{"@scope"}, imports: []string{"@scope/lib/value"}},
		{name: "unrelated namespace", external: []string{"@sc"}},
		{name: "all", all: true, imports: []string{"@scope/lib/value", "lib/value"}},
		{name: "all with explicit package", all: true, external: []string{"lib"}, imports: []string{"@scope/lib/value", "lib/value"}},
		{name: "all without bundle", all: true, mode: BundleFalse, imports: []string{"@scope/lib/value", "lib/value"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &BuildContext{
				wd: wd, target: "es2022", storage: fs, logger: logger, npmrc: &NpmRC{}, bundleMode: tt.mode, externalAll: tt.all,
				esmPath: EsmPath{PkgName: "example", PkgVersion: "1.0.0"},
				args:    BuildArgs{External: *set.NewReadOnly(tt.external...)},
				pkgJson: &npm.PackageJSON{Name: "example", Version: "1.0.0", Type: "module", Main: "./index.js", Types: "./index.d.ts", Dependencies: map[string]string{"lib": "1.0.0", "@scope/lib": "1.0.0"}},
			}
			if _, _, err := ctx.buildModule(false); err != nil {
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
			var imports []string
			result := esbuild.Build(esbuild.BuildOptions{
				Stdin: &esbuild.StdinOptions{Contents: string(output)}, Bundle: true, Format: esbuild.FormatESModule,
				Plugins: []esbuild.Plugin{{Name: "imports", Setup: func(build esbuild.PluginBuild) {
					build.OnResolve(esbuild.OnResolveOptions{Filter: ".*"}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
						imports = append(imports, args.Path)
						return esbuild.OnResolveResult{Path: args.Path, External: true}, nil
					})
				}}},
			})
			if len(result.Errors) > 0 {
				t.Fatal(result.Errors)
			}
			slices.Sort(imports)
			if !slices.Equal(imports, tt.imports) {
				t.Errorf("imports = %v, want %v", imports, tt.imports)
			}
		})
	}
}

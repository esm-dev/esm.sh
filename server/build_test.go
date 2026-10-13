package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/esm-dev/esm.sh/internal/storage"
	esbuild "github.com/ije/esbuild-internal/api"
	"github.com/ije/gox/log"
	"github.com/ije/gox/set"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
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
		{name: "globalThis process assignment", code: `if (!globalThis.process) globalThis.process = { getuid: () => -1 }; export const process = globalThis.process;`},
		{name: "global process assignment", code: `if (!global.process) global.process = { getuid: () => -1 }; export const process = global.process;`},
		{name: "bundled globalThis process assignment", code: `if (!globalThis.process) globalThis.process = { getuid: () => -1 }; export const process = globalThis.process;`, mode: BundleDeps},
		{name: "bundled global process assignment", code: `if (!global.process) global.process = { getuid: () => -1 }; export const process = global.process;`, mode: BundleDeps},
		{name: "deno globalThis process assignment", code: `if (!globalThis.process) globalThis.process = { getuid: () => -1 }; export const process = globalThis.process;`, target: "deno"},
		{name: "deno global process assignment", code: `if (!global.process) global.process = { getuid: () => -1 }; export const process = global.process;`, target: "deno"},
		{name: "denonext globalThis process assignment", code: `if (!globalThis.process) globalThis.process = { getuid: () => -1 }; export const process = globalThis.process;`, target: "denonext"},
		{name: "denonext global process assignment", code: `if (!global.process) global.process = { getuid: () => -1 }; export const process = global.process;`, target: "denonext"},
		{name: "global process env", code: `export const env = [process.env.NODE_ENV, global.process.env.NODE_ENV, globalThis.process.env.NODE_ENV];`},
		{name: "default", code: `export const env = process.env;`, imports: []string{"/node/process.mjs"}},
		{name: "no bundle", code: `export const env = process.env;`, mode: BundleFalse, imports: []string{"/node/process.mjs"}},
		{name: "external", code: `export const env = process.env; export { Buffer } from "node:buffer";`, mode: BundleDeps, external: []string{"node:process", "node:buffer"}, imports: []string{"node:buffer", "node:process"}},
		{name: "mixed external", code: `export const env = process.env; export { Buffer } from "node:buffer";`, mode: BundleDeps, external: []string{"node:buffer"}, imports: []string{"node:buffer"}},
		{name: "external all", code: `export const env = process.env; export { Buffer } from "node:buffer";`, mode: BundleDeps, externalAll: true, imports: []string{"/node/process.mjs", "node:buffer"}},
		{name: "peer", code: `export const env = process.env; export { value } from "peer";`, mode: BundleDeps, imports: []string{"/peer@1.0.0/es2022/peer.mjs"}},
		{name: "node", code: `export { default } from "node:process";`, mode: BundleDeps, target: "node", imports: []string{"node:process"}},
		{name: "denonext", code: `export const env = process.env;`, mode: BundleDeps, target: "denonext", imports: []string{"node:process"}},
		{name: "sqlite node", code: `export { DatabaseSync } from "node:sqlite";`, target: "node", imports: []string{"node:sqlite"}},
		{name: "sqlite denonext", code: `export { DatabaseSync } from "node:sqlite";`, target: "denonext", imports: []string{"node:sqlite"}},
		{name: "sqlite require", code: `export const sqlite = require("node:sqlite");`, target: "denonext", imports: []string{"node:sqlite"}},
		{name: "sqlite dynamic", code: `export const load = () => import("node:sqlite");`, mode: BundleDeps, target: "denonext", imports: []string{"node:sqlite"}},
		{name: "sqlite browser", code: `export { DatabaseSync } from "node:sqlite";`, imports: []string{"/error.js?type=unsupported-node-builtin-module&name=node:sqlite&importer=example@1.0.0"}},
		{name: "sqlite browser bundle", code: `export const sqlite = require("node:sqlite");`, mode: BundleDeps, imports: []string{"/error.js?type=unsupported-node-builtin-module&name=node:sqlite&importer=example@1.0.0"}},
		{name: "sqlite deno", code: `export { DatabaseSync } from "node:sqlite";`, target: "deno", imports: []string{"/error.js?type=unsupported-node-builtin-module&name=node:sqlite&importer=example@1.0.0"}},
		{name: "sqlite external", code: `export { DatabaseSync } from "node:sqlite";`, mode: BundleDeps, external: []string{"node:sqlite"}, imports: []string{"node:sqlite"}},
		{name: "sqlite package", code: `export { open } from "sqlite";`, mode: BundleDeps, imports: []string{"/sqlite@1.0.0/es2022/sqlite.mjs"}},
		{name: "prefix builtin", code: `export { default } from "node:test";`, mode: BundleDeps, target: "denonext", imports: []string{"node:test"}},
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
					pkgJson: &npm.PackageJSON{Name: "example", Version: "1.0.0", Type: "module", Main: "./index.js", Types: "./index.d.ts", PeerDependencies: map[string]string{"peer": "1.0.0", "sqlite": "1.0.0"}},
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

func TestBuildModuleDenoWindow(t *testing.T) {
	oldConfig := config
	config = &Config{}
	t.Cleanup(func() { config = oldConfig })
	for _, target := range []string{"deno", "denonext"} {
		for _, minify := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/minify=%v", target, minify), func(t *testing.T) {
				config.Minify = minify
				wd := t.TempDir()
				pkgDir := filepath.Join(wd, "node_modules", "example")
				if err := os.MkdirAll(pkgDir, 0755); err != nil {
					t.Fatal(err)
				}
				const code = `export const windowType = typeof window;
export const globalWindowType = typeof globalThis.window;
export const browser = typeof window !== "undefined" && window.document;`
				if err := os.WriteFile(filepath.Join(pkgDir, "index.js"), []byte(code), 0644); err != nil {
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
					wd: wd, target: target, storage: fs, logger: logger, npmrc: &NpmRC{},
					esmPath: EsmPath{PkgName: "example", PkgVersion: "1.0.0"},
					pkgJson: &npm.PackageJSON{Name: "example", Version: "1.0.0", Type: "module", Main: "./index.js", Types: "./index.d.ts"},
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
				for _, expr := range []string{"typeof window", "typeof globalThis.window", "window.document"} {
					if !strings.Contains(string(output), expr) {
						t.Errorf("missing %q in output:\n%s", expr, output)
					}
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

func TestBuildModuleSharedCSS(t *testing.T) {
	for _, target := range []string{"es2022", "denonext"} {
		for _, cached := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/cached=%v", target, cached), func(t *testing.T) {
				wd := t.TempDir()
				pkgDir := filepath.Join(wd, "node_modules", "example")
				if err := os.MkdirAll(pkgDir, 0755); err != nil {
					t.Fatal(err)
				}
				for name, code := range map[string]string{
					"a.js":       `import "./shared.css"; export { value } from "./shared.js";`,
					"b.js":       `import "./shared.css"; export { value } from "./shared.js";`,
					"shared.js":  `export const value = 1;`,
					"shared.css": `.shared { color: red; }`,
				} {
					if err := os.WriteFile(filepath.Join(pkgDir, name), []byte(code), 0644); err != nil {
						t.Fatal(err)
					}
				}
				if cached {
					if err := os.WriteFile(filepath.Join(wd, "splitting.txt"), []byte("2\nshared.js\nshared.css"), 0644); err != nil {
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
				ctx := &BuildContext{
					wd: wd, target: target, storage: fs, logger: logger, npmrc: &NpmRC{},
					esmPath: EsmPath{PkgName: "example", PkgVersion: "1.0.0", SubPath: "a"},
					pkgJson: &npm.PackageJSON{
						Name: "example", Version: "1.0.0", Type: "module", Types: "./index.d.ts",
						SideEffects: *set.NewReadOnly("*.css"),
						Exports:     npm.NewJSONObject([]string{"./a", "./b"}, map[string]any{"./a": "./a.js", "./b": "./b.js"}),
					},
				}
				ctx.analyzeSplitting()
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
				if strings.Contains(string(output), ".css") {
					t.Errorf("JavaScript imports CSS: %s", output)
				}
				wantImports := []string{"/example@1.0.0/" + target + "/shared.mjs"}
				if !slices.Equal(meta.Imports, wantImports) {
					t.Errorf("imports = %v, want %v", meta.Imports, wantImports)
				}
				if !meta.CSSInJS {
					t.Fatal("missing CSS output metadata")
				}
				f, _, err = fs.Get(strings.TrimSuffix(ctx.getSavePath(), ".mjs") + ".css")
				if err != nil {
					t.Fatal(err)
				}
				defer f.Close()
				css, err := io.ReadAll(f)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(css), ".shared") || !strings.Contains(string(css), "red") {
					t.Errorf("missing shared styles: %s", css)
				}
			})
		}
	}
}

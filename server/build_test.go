package server

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/esm-dev/esm.sh/internal/storage"
	esbuild "github.com/ije/esbuild-internal/api"
	"github.com/ije/gox/log"
	"github.com/ije/gox/set"
)

func TestBuildModuleConditionalExportSplitting(t *testing.T) {
	for _, tt := range []struct {
		name    string
		exports string
		subPath string
		split   bool
	}{
		{"default", `{"import":"./public.mjs","default":"./internal.cjs"}`, "feature", false},
		{"require", `{"import":"./public.mjs","require":"./internal.cjs"}`, "feature", false},
		{"nested", `{"import":{"default":"./public.mjs"},"require":{"default":"./internal.cjs"}}`, "feature", false},
		{"import", `{"import":"./internal.cjs","default":"./public.mjs"}`, "feature", true},
		{"root wrapper", `{"import":"./public.mjs","require":"./internal.cjs"}`, "", false},
		{"subpath wrapper", `{"import":"./public.mjs","require":"./internal.cjs"}`, "core", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			wd := t.TempDir()
			pkgDir := filepath.Join(wd, "node_modules", "example")
			if err := os.MkdirAll(pkgDir, 0755); err != nil {
				t.Fatal(err)
			}
			for name, code := range map[string]string{
				"feature.mjs":  `import core from "./internal.cjs"; export const answer = core.hidden.answer;`,
				"feature.d.ts": `export declare const answer: number;`,
				"internal.cjs": `module.exports = { hidden: { answer: 42 } };`,
				"public.mjs":   `import core from "./internal.cjs"; export const answer = core.hidden.answer;`,
			} {
				if err := os.WriteFile(filepath.Join(pkgDir, name), []byte(code), 0644); err != nil {
					t.Fatal(err)
				}
			}
			exportName := "./core"
			if tt.subPath == "" {
				exportName = "."
			}
			var raw npm.PackageJSONRaw
			if err := json.Unmarshal([]byte(`{"name":"example","version":"1.0.0","types":"./feature.d.ts","exports":{"./feature":"./feature.mjs","`+exportName+`":`+tt.exports+`}}`), &raw); err != nil {
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
				wd: wd, target: "denonext", storage: fs, logger: logger,
				esmPath: EsmPath{PkgName: "example", PkgVersion: "1.0.0", SubPath: tt.subPath},
				pkgJson: raw.ToNpmPackage(),
			}
			if tt.subPath != "feature" {
				ctx.pkgJson.SideEffects = *set.NewReadOnly("./side-effect.js")
			}
			meta, _, err := ctx.buildModule(false)
			if err != nil {
				t.Fatal(err)
			}
			var wantImports []string
			if tt.split {
				wantImports = []string{"/example@1.0.0/denonext/core.mjs"}
			}
			if !slices.Equal(meta.Imports, wantImports) {
				t.Fatalf("imports = %v, want %v", meta.Imports, wantImports)
			}
		})
	}
}

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
		{name: "exact subpath", external: []string{"lib/value"}, imports: []string{"lib/value"}},
		{name: "subpath wildcard", external: []string{"lib/*"}, imports: []string{"lib/value"}},
		{name: "subpath wildcard with bundle", mode: BundleDeps, external: []string{"lib/*"}, imports: []string{"lib/value"}},
		{name: "scoped subpath wildcard", external: []string{"@scope/lib/*"}, imports: []string{"@scope/lib/value"}},
		{name: "scoped subpath wildcard with bundle", mode: BundleDeps, external: []string{"@scope/lib/*"}, imports: []string{"@scope/lib/value"}},
		{name: "subpath wildcard boundary", external: []string{"lib/value/*", "@scope/lib/value/*"}},
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

func TestBuildModuleDependencyRootVersion(t *testing.T) {
	for _, name := range []string{"example", "@scope/example"} {
		for _, tt := range []struct {
			version, subpath, external string
		}{
			{version: "2.0.0"},
			{version: "2.0.0", subpath: "/component"},
			{version: "^2.0.0", subpath: "/component"},
			{version: "^1.0.0", subpath: "/component"},
			{version: "2.0.0", subpath: "/component", external: name},
			{version: "2.0.0", subpath: "/component", external: name + "/*"},
			{version: "2.0.0", subpath: "/component", external: name + "/component"},
		} {
			t.Run(fmt.Sprintf("%s%s@%s/external=%v", name, tt.subpath, tt.version, tt.external), func(t *testing.T) {
				wd := t.TempDir()
				for filename, content := range map[string]string{
					name + "/package.json": `{"name":"` + name + `","version":"1.0.0","type":"module","main":"index.js"}`,
					name + "/index.js":     `export { value } from "shim";`,
					name + "/component.js": `export const value = 42;`,
					"shim/package.json":    `{"name":"shim","version":"1.0.0","type":"module","main":"index.js","dependencies":{"` + name + `":"` + tt.version + `"}}`,
					"shim/index.js":        `export { value } from "` + name + tt.subpath + `";`,
				} {
					if tt.external != "" && filename == name+"/component.js" {
						continue
					}
					filename = filepath.Join(wd, "node_modules", filename)
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
				ctx := &BuildContext{
					wd: wd, target: "es2022", bundleMode: BundleDeps, storage: fs, logger: logger, npmrc: &NpmRC{},
					esmPath: EsmPath{PkgName: name, PkgVersion: "1.0.0"},
					pkgJson: &npm.PackageJSON{Name: name, Version: "1.0.0", Type: "module", Main: "./index.js", Types: "./index.d.ts", Dependencies: map[string]string{"shim": "1.0.0"}},
				}
				if tt.external != "" {
					ctx.args.External = *set.NewReadOnly(tt.external)
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
				entry := "example"
				if tt.subpath != "" {
					entry = "component"
				}
				want := "/" + name + "@2.0.0/es2022/" + entry + ".mjs"
				wantImports := []string{want}
				switch {
				case tt.external != "":
					want, wantImports = name+tt.subpath, nil
				case tt.version == "^2.0.0":
					want = "/" + name + "@^2.0.0/component?target=es2022"
					wantImports = []string{want}
				case tt.version == "^1.0.0":
					want, wantImports = "42", nil
				}
				if !slices.Equal(meta.Imports, wantImports) || !strings.Contains(string(output), want) {
					t.Fatalf("expected %s with imports %v, got %v:\n%s", want, wantImports, meta.Imports, output)
				}
			})
		}
	}
}

func TestBuildModuleCJSNamedExports(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires shell scripts")
	}
	oldConfig := config
	config = &Config{WorkDir: t.TempDir()}
	t.Cleanup(func() {
		config = oldConfig
		onceMap.Delete("install-cjs-module-lexer")
		onceMap.Delete("check-deno")
	})
	binDir := filepath.Join(config.WorkDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	names := []string{"eval", "arguments", "class", "await", "cjsm", "valid", "__esModule", "default", "not-an-identifier"}
	encoded, err := json.Marshal(names)
	if err != nil {
		t.Fatal(err)
	}
	for filename, script := range map[string]string{
		"cjs-module-lexer-" + cjsModuleLexerVersion: "#!/bin/sh\ncat <<'EOF'\n" + strings.Join(names, "\n") + "\nEOF\n",
		"deno": "#!/bin/sh\nif [ \"$1\" = eval ]; then echo " + denoVersion + "; else echo '" + string(encoded) + "'; fi\n",
	} {
		if err := os.WriteFile(filepath.Join(binDir, filename), []byte(script), 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, pkgName := range []string{"example", "pako"} {
		for _, minify := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/minify=%v", pkgName, minify), func(t *testing.T) {
				config.Minify = minify
				wd := t.TempDir()
				pkgDir := filepath.Join(wd, "node_modules", pkgName)
				if err := os.MkdirAll(pkgDir, 0755); err != nil {
					t.Fatal(err)
				}
				var code strings.Builder
				for i, name := range names {
					fmt.Fprintf(&code, "exports[%q] = %d;\n", name, i+1)
				}
				if err := os.WriteFile(filepath.Join(pkgDir, "index.js"), []byte(code.String()), 0644); err != nil {
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
					esmPath: EsmPath{PkgName: pkgName, PkgVersion: "1.0.0"},
					pkgJson: &npm.PackageJSON{Name: pkgName, Version: "1.0.0", Main: "./index.js", Types: "./index.d.ts"},
				}
				cacheDir := filepath.Join(wd, ".cjs-module-lexer")
				if err := os.MkdirAll(cacheDir, 0755); err != nil {
					t.Fatal(err)
				}
				cacheVersions := []string{cjsModuleLexerVersion}
				if pkgName == "pako" {
					cacheVersions = append(cacheVersions, cjsModuleLexerVersion+"-2", "deno-"+denoVersion)
				}
				for _, version := range cacheVersions {
					oldHash := sha1.Sum([]byte(version + "./index.js" + ctx.getNodeEnv()))
					oldCache := filepath.Join(cacheDir, base64.RawURLEncoding.EncodeToString(oldHash[:])+".json")
					if err := os.WriteFile(oldCache, []byte(`{"exports":["valid"]}`), 0644); err != nil {
						t.Fatal(err)
					}
				}
				lexed, err := cjsModuleLexer(ctx, "./index.js")
				if err != nil {
					t.Fatal(err)
				}
				if !slices.Equal(lexed.Exports, names[:7]) {
					t.Fatalf("lexer exports = %v, want %v", lexed.Exports, names[:7])
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
				filename := filepath.Join(wd, "output.mjs")
				if err := os.WriteFile(filename, output, 0644); err != nil {
					t.Fatal(err)
				}
				_, exports, err := validateJSFile(filename)
				if err != nil {
					t.Fatal(err)
				}
				want := slices.Clone(names[:8])
				slices.Sort(want)
				slices.Sort(exports)
				if !slices.Equal(exports, want) {
					t.Fatalf("module exports = %v, want %v", exports, want)
				}
			})
		}
	}
}

func TestBuildInstallDependencyOverrides(t *testing.T) {
	oldConfig, transport := config, http.DefaultTransport
	config = &Config{WorkDir: t.TempDir()}
	t.Cleanup(func() { config, http.DefaultTransport = oldConfig, transport })
	npmrc := &NpmRC{globalRegistry: &NpmRegistry{NpmRegistryConfig: NpmRegistryConfig{Registry: npmRegistry}}}
	http.DefaultTransport = ghTestTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})
	for name, contents := range map[string]string{
		"install-root@1.0.0":   `{"name":"install-root","version":"1.0.0","dependencies":{"install-direct":"0.0.0"}}`,
		"install-direct@1.0.0": `{"name":"install-direct","version":"1.0.0","dependencies":{"install-leaf":"0.0.0"}}`,
		"install-direct@2.0.0": `{"name":"install-direct","version":"2.0.0","dependencies":{"install-leaf":"0.0.0"}}`,
		"install-leaf@1.0.0":   `{"name":"install-leaf","version":"1.0.0"}`,
		"install-leaf@2.0.0":   `{"name":"install-leaf","version":"2.0.0"}`,
	} {
		pkgDir := filepath.Join(npmrc.StoreDir(), name, "node_modules", strings.Split(name, "@")[0])
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(contents), 0644); err != nil {
			t.Fatal(err)
		}
	}
	var builds []*BuildContext
	for _, version := range []string{"1.0.0", "2.0.0", "1.0.0"} {
		ctx := &BuildContext{
			npmrc: npmrc, bundleMode: BundleDeps,
			esmPath: EsmPath{PkgName: "install-root", PkgVersion: "1.0.0"},
			args:    BuildArgs{Deps: map[string]string{"install-direct": version, "install-leaf": version}},
		}
		if err := ctx.install(); err != nil {
			t.Errorf("version=%s: %v", version, err)
			continue
		}
		builds = append(builds, ctx)
	}
	for _, ctx := range builds {
		for name, version := range ctx.args.Deps {
			data, err := os.ReadFile(filepath.Join(ctx.wd, "node_modules", name, "package.json"))
			if err != nil {
				t.Fatal(err)
			}
			var pkg npm.PackageJSON
			if err := json.Unmarshal(data, &pkg); err != nil {
				t.Fatal(err)
			}
			if pkg.Version != version {
				t.Errorf("%s version = %s, want %s", name, pkg.Version, version)
			}
		}
	}
}

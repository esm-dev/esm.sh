package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/esm-dev/esm.sh/internal/npm"
	esbuild "github.com/ije/esbuild-internal/api"
	"github.com/ije/gox/set"
)

func TestWasmInstanceDependencyArgs(t *testing.T) {
	for _, tt := range []struct {
		version  string
		instance bool
		want     string
	}{
		{"1.0.0", false, "/dependency@1.0.0/es2022/dependency.mjs"},
		{"1.0.0", true, "/dependency@1.0.0/X-dw/es2022/dependency.mjs"},
		{"^1.0.0", false, "/dependency@^1.0.0?target=es2022"},
		{"^1.0.0", true, "/dependency@^1.0.0?wasm=instance&target=es2022"},
	} {
		t.Run(fmt.Sprintf("%s/%t", tt.version, tt.instance), func(t *testing.T) {
			ctx := &BuildContext{target: "es2022", esmPath: EsmPath{PkgName: "example", PkgVersion: "1.0.0"},
				pkgJson: &npm.PackageJSON{Name: "example", Version: "1.0.0", Dependencies: map[string]string{"dependency": tt.version}}, args: BuildArgs{WasmInstance: tt.instance}}
			got, _, err := ctx.resolveExternalModule("dependency", esbuild.ResolveJSImportStatement, "", false)
			if err != nil || got != tt.want {
				t.Fatalf("resolved = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestBuildModuleWasmInstances(t *testing.T) {
	denoPath, err := exec.LookPath("deno")
	if err != nil {
		t.Skip("requires Deno")
	}
	oldConfig, oldNpmRC := config, defaultNpmRC
	once, hadOnce := onceMap.Load("check-deno")
	config = &Config{WorkDir: t.TempDir(), NpmRegistry: npmRegistry, NpmQueryCacheTTL: 60}
	defaultNpmRC = nil
	t.Cleanup(func() {
		config, defaultNpmRC = oldConfig, oldNpmRC
		if hadOnce {
			onceMap.Store("check-deno", once)
		} else {
			onceMap.Delete("check-deno")
		}
	})
	// Exercise the installed runtime (including the Deno 1 CI leg) without downloading another version.
	if err := os.MkdirAll(filepath.Dir(resolveDenoPath(config.WorkDir)), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(denoPath, resolveDenoPath(config.WorkDir)); err != nil {
		t.Fatal(err)
	}
	onceMap.Delete("check-deno")
	if err := doOnce("check-deno", func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	t.Run("start", func(t *testing.T) {
		filename := filepath.Join(t.TempDir(), "start.wasm")
		// The start function traps, so compilation must succeed and instantiation must fail.
		wasm := []byte{0, 97, 115, 109, 1, 0, 0, 0, 1, 4, 1, 96, 0, 0, 3, 2, 1, 0, 8, 1, 0, 10, 5, 1, 3, 0, 0, 11}
		if err := os.WriteFile(filename, wasm, 0644); err != nil {
			t.Fatal(err)
		}
		output, err := transformWasm(t.Context(), &NpmRC{}, filename)
		if err != nil {
			t.Fatal(err)
		}
		code := fmt.Sprintf(`try { await import(%q); throw Error("start did not trap"); } catch (err) { if (!(err instanceof WebAssembly.RuntimeError)) throw err; }`, "data:application/javascript;base64,"+base64.StdEncoding.EncodeToString([]byte(output.Code)))
		if output, err := exec.CommandContext(t.Context(), resolveDenoPath(config.WorkDir), "eval", "--no-config", "--no-lock", code).CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, output)
		}
	})
	for name, field := range map[string]string{"main": `"main":"./module.wasm"`, "exports": `"exports":{".":{"default":"./module.wasm"}}`, "wildcard": `"exports":{"./parts/*":"./*.wasm"}`} {
		for _, instance := range []bool{false, true} {
			t.Run(fmt.Sprintf("entry/%s/%t", name, instance), func(t *testing.T) {
				wd := t.TempDir()
				pkgDir := filepath.Join(wd, "node_modules", "example")
				if err := os.MkdirAll(pkgDir, 0755); err != nil {
					t.Fatal(err)
				}
				manifest := `{"name":"example","version":"1.0.0","types":"./index.d.ts",` + field + `}`
				for name, data := range map[string][]byte{"package.json": []byte(manifest), "module.wasm": {0, 97, 115, 109, 1, 0, 0, 0}} {
					if err := os.WriteFile(filepath.Join(pkgDir, name), data, 0644); err != nil {
						t.Fatal(err)
					}
				}
				var raw npm.PackageJSONRaw
				if err := json.Unmarshal([]byte(manifest), &raw); err != nil {
					t.Fatal(err)
				}
				fs, _, logger := newPurgeTestEnv(t)
				ctx := &BuildContext{wd: wd, target: "es2022", storage: fs, logger: logger, npmrc: &NpmRC{},
					esmPath: EsmPath{PkgName: "example", PkgVersion: "1.0.0"}, pkgJson: raw.ToNpmPackage(), args: BuildArgs{WasmInstance: instance}}
				if name == "wildcard" {
					ctx.esmPath.SubPath = "parts/module"
				} else {
					base, _ := url.Parse("https://esm.sh" + ctx.Path())
					for mode, query := range map[string]string{"bytes": "bytes", "wasm-module": "module"} {
						got, _, err := ctx.resolveExternalModule("example", esbuild.ResolveJSImportStatement, mode, false)
						resolved, parseErr := url.Parse(got)
						if err != nil || parseErr != nil || base.ResolveReference(resolved).String() != "https://esm.sh/example@1.0.0/module.wasm?"+query {
							t.Fatalf("self-reference %s = %q, %v", mode, got, err)
						}
					}
				}
				if entry := ctx.resolveEntry(ctx.esmPath); !entry.module {
					t.Fatalf("WASM entry must use the ESM loader: %+v", entry)
				}
				meta, _, err := ctx.buildModule(false)
				if err != nil {
					t.Fatal(err)
				}
				if meta.ExportDefault == instance || meta.CJS {
					t.Fatalf("unexpected WASM entry metadata: %+v", meta)
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
				code := fmt.Sprintf(`import * as value from %q; if (%t ? Object.keys(value).length !== 0 : !(value.default instanceof Uint8Array)) throw Error("wrong WASM entry");`, "data:application/javascript;base64,"+base64.StdEncoding.EncodeToString(output), instance)
				if output, err := exec.CommandContext(t.Context(), resolveDenoPath(config.WorkDir), "eval", "--no-config", "--no-lock", code).CombinedOutput(); err != nil {
					t.Fatalf("%v\n%s", err, output)
				}
			})
		}
	}
	for _, target := range []string{"es2022", "denonext"} {
		for _, mode := range []BundleMode{BundleDefault, BundleDeps, BundleFalse} {
			t.Run(fmt.Sprintf("%s/%d", target, mode), func(t *testing.T) {
				wd := t.TempDir()
				pkgDir := filepath.Join(wd, "node_modules", "example")
				if err := os.MkdirAll(pkgDir, 0755); err != nil {
					t.Fatal(err)
				}
				manifest := `{"name":"example","version":"1.0.0","type":"module","main":"./index.js","types":"./index.d.ts","exports":{".":"./index.js","./second":"./second.js"}}`
				for name, code := range map[string]string{
					"package.json": manifest,
					"index.js":     `export * from "./module.wasm"; export { default as bytes } from "./module.wasm" with { type: "bytes" }; export { default as compiled } from "./module.wasm?module"; export const load = () => import("./module.wasm"); export const loadBytes = () => import("./module.wasm", { with: { type: "bytes" } }); export const loadModule = () => import("./module.wasm?module");`,
					"second.js":    `export * from "./module.wasm";`,
					"glue.js":      `import { memory } from "./module.wasm"; export function next() { return ++new Uint8Array(memory.buffer)[0]; }`,
				} {
					if err := os.WriteFile(filepath.Join(pkgDir, name), []byte(code), 0644); err != nil {
						t.Fatal(err)
					}
				}
				// The fixture imports next from glue.js; its start function increments memory[1].
				wasm, err := base64.StdEncoding.DecodeString("AGFzbQEAAAABCAJgAAF/YAAAAhIBCS4vZ2x1ZS5qcwRuZXh0AAADAwIAAQQEAXAAAQUDAQABBgYBfwBBBwsHIgQEbmV4dAABBm1lbW9yeQIABXRhYmxlAQAGZ2xvYmFsAwAIAQIKFgIEABAACw8AQQFBAS0AAEEBajoAAAs=")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(pkgDir, "module.wasm"), wasm, 0644); err != nil {
					t.Fatal(err)
				}
				rawDir := filepath.Join(config.WorkDir, "npm", "example@1.0.0", "node_modules", "example")
				if err := os.MkdirAll(rawDir, 0755); err != nil {
					t.Fatal(err)
				}
				for name, data := range map[string][]byte{"package.json": []byte(manifest), "module.wasm": wasm} {
					if err := os.WriteFile(filepath.Join(rawDir, name), data, 0644); err != nil {
						t.Fatal(err)
					}
				}
				fs, _, logger := newPurgeTestEnv(t)
				var raw npm.PackageJSONRaw
				if err := json.Unmarshal([]byte(manifest), &raw); err != nil {
					t.Fatal(err)
				}
				outputs := map[string][]byte{}
				paths := map[string]string{}
				for _, subpath := range []string{"", "second", "glue", "module.wasm"} {
					ctx := &BuildContext{wd: wd, target: target, bundleMode: mode, storage: fs, logger: logger, npmrc: &NpmRC{},
						esmPath: EsmPath{PkgName: "example", PkgVersion: "1.0.0", SubPath: subpath}, pkgJson: raw.ToNpmPackage(), args: BuildArgs{WasmInstance: true}}
					ctx.analyzeSplitting()
					if _, _, err := ctx.buildModule(false); err != nil {
						t.Fatalf("%s: %v", subpath, err)
					}
					f, _, err := fs.Get(ctx.getSavePath())
					if err != nil {
						t.Fatal(err)
					}
					outputs[ctx.Path()], err = io.ReadAll(f)
					f.Close()
					if err != nil {
						t.Fatal(err)
					}
					paths[subpath] = ctx.Path()
				}
				router := esmRouter(fs, logger)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if code, ok := outputs[r.URL.Path]; ok {
						w.Header().Set("Content-Type", "application/javascript")
						w.Write(code)
					} else {
						router.ServeHTTP(w, r)
					}
				}))
				defer server.Close()
				code := fmt.Sprintf(`import * as a from %q; import * as b from %q;
				if (!(a.bytes instanceof Uint8Array) || !(a.compiled instanceof WebAssembly.Module)) throw Error("lost byte/module imports");
				if ((await a.loadBytes()).default !== a.bytes || (await a.loadModule()).default !== a.compiled) throw Error("dynamic import created another bytes/module value");
				if (!(a.memory instanceof WebAssembly.Memory) || !(a.table instanceof WebAssembly.Table) || a.global.value !== 7) throw Error("lost exports");
				if (a.next() !== 1 || (await a.load()).next() !== 2) throw Error("dynamic import created another instance");
				if (new Uint8Array(a.memory.buffer)[1] !== 1) throw Error("start did not run exactly once");
				if (%t && (a.memory !== b.memory || b.next() !== 3)) throw Error("entries created different instances");`, server.URL+paths[""], server.URL+paths["second"], mode != BundleDeps)
				if output, err := exec.CommandContext(t.Context(), resolveDenoPath(config.WorkDir), "eval", "--no-config", "--no-lock", code).CombinedOutput(); err != nil {
					t.Fatalf("%v\n%s\n%s", err, output, outputs[paths[""]])
				}
			})
		}
	}
	t.Run("imports", testWasmImportModes)
}

func testWasmImportModes(t *testing.T) {
	const dependency = "wasm-mode-dependency"
	key := DefaultNpmRC().resolutionFreshKey(dependency, "npm:"+dependency+"@^1.0.0")
	setCacheItem(key, &npm.PackageJSON{Name: dependency, Version: "1.0.0"}, time.Minute)
	t.Cleanup(func() { deleteCacheItem(key) })
	for _, source := range []string{"", "1.0.0", "^1.0.0", "imports", "exports"} {
		version := source
		if source == "imports" || source == "exports" {
			version = ""
		}
		for _, instance := range []bool{false, true} {
			for _, mode := range []BundleMode{BundleDefault, BundleDeps, BundleFalse} {
				t.Run(fmt.Sprintf("%s/%t/%d", source, instance, mode), func(t *testing.T) {
					wd := t.TempDir()
					prefix, wasmPackage := ".", "example"
					if version != "" {
						prefix, wasmPackage = dependency, dependency
					}
					manifest := `{"name":"example","version":"1.0.0","type":"module","main":"./index.js","types":"./index.d.ts","exports":{".":"./index.js","./second":"./second.js"}}`
					if source == "imports" {
						manifest = strings.TrimSuffix(manifest, "}") + `,"imports":{"#compiled":"./module.wasm?module","#bytes":"./module.wasm"}}`
					} else if source == "exports" {
						manifest = strings.Replace(manifest, `"./second":"./second.js"`, `"./second":"./second.js","./parts/*":"./*.wasm"`, 1)
					}
					var raw npm.PackageJSONRaw
					if err := json.Unmarshal([]byte(manifest), &raw); err != nil {
						t.Fatal(err)
					}
					pkg := raw.ToNpmPackage()
					if version != "" {
						pkg.Dependencies = map[string]string{dependency: version}
					}
					imports := fmt.Sprintf(`export * as plain from "%s/module.wasm"; export { default as bytes } from "%s/module.wasm" with { type: "bytes" }; export { default as compiled } from "%s/module.wasm?module";`, prefix, prefix, prefix)
					dynamic := fmt.Sprintf(`export const load = () => import("%s/module.wasm"); export const loadBytes = () => import("%s/module.wasm", { with: { type: "bytes" } }); export const loadModule = () => import("%s/module.wasm?module"); export const loadOnlyBytes = () => import("%s/other.wasm", { with: { type: "bytes" } }); export const loadOnlyModule = () => import("%s/other.wasm?module");`, prefix, prefix, prefix, prefix, prefix)
					if source == "imports" {
						mapping := strings.NewReplacer("./module.wasm?module", "#compiled", "./module.wasm", "#bytes")
						imports, dynamic = mapping.Replace(imports), mapping.Replace(dynamic)
					} else if source == "exports" {
						mapping := strings.NewReplacer(`"./module.wasm"`, `"example/parts/module"`, `"./other.wasm"`, `"example/parts/other"`)
						imports, dynamic = mapping.Replace(imports), mapping.Replace(dynamic)
					}
					files := map[string][]byte{
						"example/package.json": []byte(manifest), "example/index.js": []byte(imports + dynamic), "example/second.js": []byte(imports),
						wasmPackage + "/module.wasm": {0, 97, 115, 109, 1, 0, 0, 0}, wasmPackage + "/other.wasm": {0, 97, 115, 109, 1, 0, 0, 0},
					}
					if version != "" {
						files[dependency+"/package.json"] = []byte(`{"name":"` + dependency + `","version":"1.0.0"}`)
					}
					for name, data := range files {
						pkgName, _, _ := strings.Cut(name, "/")
						for _, root := range []string{wd, filepath.Join(config.WorkDir, "npm", pkgName+"@1.0.0")} {
							filename := filepath.Join(root, "node_modules", name)
							if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
								t.Fatal(err)
							}
							if err := os.WriteFile(filename, data, 0644); err != nil {
								t.Fatal(err)
							}
						}
					}
					fs, _, logger := newPurgeTestEnv(t)
					outputs := map[string][]byte{}
					paths := map[string]string{}
					for _, subpath := range []string{"", "second", "module.wasm"} {
						ctx := &BuildContext{wd: wd, target: "es2022", bundleMode: mode, storage: fs, logger: logger, npmrc: &NpmRC{},
							esmPath: EsmPath{PkgName: "example", PkgVersion: "1.0.0", SubPath: subpath}, pkgJson: pkg, args: BuildArgs{WasmInstance: instance}}
						if subpath == "module.wasm" && version != "" {
							ctx.esmPath.PkgName = dependency
							ctx.pkgJson = &npm.PackageJSON{Name: dependency, Version: "1.0.0", Types: "./index.d.ts"}
							ctx.bundleMode = BundleDefault
						}
						ctx.analyzeSplitting()
						if _, _, err := ctx.buildModule(false); err != nil {
							t.Fatal(err)
						}
						f, _, err := fs.Get(ctx.getSavePath())
						if err != nil {
							t.Fatal(err)
						}
						outputs[ctx.Path()], err = io.ReadAll(f)
						f.Close()
						if err != nil {
							t.Fatal(err)
						}
						if mode != BundleDeps && subpath != "module.wasm" && strings.Contains(string(outputs[ctx.Path()]), "AGFzb") {
							t.Fatalf("expected shared external WASM imports: %s", outputs[ctx.Path()])
						}
						paths[subpath] = ctx.Path()
					}
					router := esmRouter(fs, logger)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if code, ok := outputs[r.URL.Path]; ok {
							w.Header().Set("Content-Type", ctJavaScript)
							w.Write(code)
						} else {
							router.ServeHTTP(w, r)
						}
					}))
					defer server.Close()
					code := fmt.Sprintf(`const fetched = []; const fetch = globalThis.fetch; globalThis.fetch = (url, ...args) => { fetched.push(String(url)); return fetch(url, ...args); };
					const a = await import(%q); const b = await import(%q);
					if (%t ? Object.keys(a.plain).length !== 0 : !(a.plain.default instanceof Uint8Array)) throw Error("wrong unannotated WASM mode");
					if ((await a.load()) !== a.plain) throw Error("unannotated dynamic import lost identity");
					if (!(a.bytes instanceof Uint8Array) || !(a.compiled instanceof WebAssembly.Module)) throw Error("wrong explicit WASM modes");
					if ((await a.loadBytes()).default !== a.bytes || (await a.loadModule()).default !== a.compiled) throw Error("dynamic imports lost identity");
					if (fetched.some(url => url.endsWith("/other.wasm"))) throw Error("dynamic-only import was eager");
					if (!((await a.loadOnlyBytes()).default instanceof Uint8Array) || !((await a.loadOnlyModule()).default instanceof WebAssembly.Module)) throw Error("dynamic-only imports lost mode");
					if (%t && (a.plain !== b.plain || a.bytes !== b.bytes || a.compiled !== b.compiled)) throw Error("entries lost shared wrappers");
					try { await import(%q); throw Error("missing WASM returned bytes"); } catch (err) { if (!String(err).includes("Failed to fetch WASM: 404")) throw err; }`, server.URL+paths[""], server.URL+paths["second"], instance, mode != BundleDeps, server.URL+"/example@1.0.0/missing.wasm?bytes")
					if output, err := exec.CommandContext(t.Context(), resolveDenoPath(config.WorkDir), "eval", "--no-config", "--no-lock", code).CombinedOutput(); err != nil {
						t.Fatalf("%v\n%s\n%s", err, output, outputs[paths[""]])
					}
				})
			}
		}
	}
}

func TestBuildModuleWasmExternal(t *testing.T) {
	oldConfig := config
	config = &Config{WorkDir: t.TempDir()}
	t.Cleanup(func() { config = oldConfig })
	for _, mode := range []BundleMode{BundleDefault, BundleDeps, BundleFalse} {
		for _, externalAll := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/%t", mode, externalAll), func(t *testing.T) {
				wd := t.TempDir()
				for name, data := range map[string][]byte{
					"example/index.js":       []byte(`export { default as compiled } from "dependency/module.wasm?module"; export { default as bytes } from "dependency/module.wasm" with { type: "bytes" }; export { value } from "dependency/value.js";`),
					"dependency/module.wasm": {0, 97, 115, 109, 1, 0, 0, 0},
					"dependency/value.js":    []byte(`export const value = 1`),
				} {
					filename := filepath.Join(wd, "node_modules", name)
					if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filename, data, 0644); err != nil {
						t.Fatal(err)
					}
				}
				fs, _, logger := newPurgeTestEnv(t)
				args := BuildArgs{WasmInstance: true}
				if !externalAll {
					args.External = *set.NewReadOnly("dependency")
				}
				ctx := &BuildContext{wd: wd, target: "esnext", bundleMode: mode, externalAll: externalAll, storage: fs, logger: logger, npmrc: &NpmRC{},
					esmPath: EsmPath{PkgName: "example", PkgVersion: "1.0.0"}, args: args,
					pkgJson: &npm.PackageJSON{Name: "example", Version: "1.0.0", Type: "module", Main: "./index.js", Types: "./index.d.ts"}}
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
				if !strings.Contains(string(output), `"dependency/module.wasm?module"`) || !strings.Contains(string(output), `"dependency/module.wasm"`) || !strings.Contains(string(output), `"dependency/value.js"`) || strings.Contains(string(output), "AGFzb") {
					t.Fatalf("expected WASM imports to remain external: %s", output)
				}
				module, err := parseJS("output.mjs", output)
				if err != nil || len(module.ImportRecords) != 3 {
					t.Fatalf("unexpected imports: %v\n%s", err, output)
				}
				for _, record := range module.ImportRecords {
					if record.Path.Text == "dependency/module.wasm" {
						attrs := record.AssertOrWith
						if attrs == nil || len(attrs.Entries) != 1 || string(utf16.Decode(attrs.Entries[0].Key)) != "type" || string(utf16.Decode(attrs.Entries[0].Value)) != "bytes" {
							t.Fatalf("external bytes import lost its attribute: %s", output)
						}
					}
				}
			})
		}
	}
}

package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/esm-dev/esm.sh/internal/storage"
	"github.com/ije/gox/log"
)

func TestResolveEntryTypesOnlyJSSubPath(t *testing.T) {
	wd := t.TempDir()
	dir := filepath.Join(wd, "node_modules", "@types", "example")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "foo.d.ts"), []byte("export declare const foo: string;"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx := &BuildContext{
		wd: wd, target: "types",
		esmPath: EsmPath{PkgName: "@types/example", PkgVersion: "1.0.0"},
		pkgJson: &npm.PackageJSON{Name: "@types/example", Version: "1.0.0"},
	}
	for _, subPath := range []string{"foo.js", "foo.mjs", "foo.cjs"} {
		esm := ctx.esmPath
		esm.SubPath = subPath
		if entry := ctx.resolveEntry(esm); entry.types != "./foo.d.ts" {
			t.Errorf("%s: types = %q, want ./foo.d.ts", subPath, entry.types)
		}
	}
}

func TestBuildModuleExtensionlessExportTarget(t *testing.T) {
	for _, export := range []string{`"./lib/state"`, `{"import":"./lib/state"}`} {
		t.Run(export, func(t *testing.T) {
			wd := t.TempDir()
			for name, content := range map[string]string{
				"index.mjs":     `export { state } from "./lib/state.mjs";`,
				"index.d.ts":    `export declare const state: object;`,
				"lib/state.mjs": `export const state = {};`,
			} {
				filename := filepath.Join(wd, "node_modules", "example", name)
				if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}
			var raw npm.PackageJSONRaw
			if err := json.Unmarshal([]byte(`{"name":"example","version":"1.0.0","type":"module","types":"./index.d.ts","exports":{".":"./index.mjs","./state":`+export+`}}`), &raw); err != nil {
				t.Fatal(err)
			}
			fs, err := storage.NewFSStorage(filepath.Join(wd, "storage"))
			if err != nil {
				t.Fatal(err)
			}
			logger := new(log.Logger)
			logger.SetOutput(io.Discard)
			ctx := &BuildContext{
				wd: wd, target: "es2022", storage: fs, logger: logger, npmrc: &NpmRC{},
				esmPath: EsmPath{PkgName: "example", PkgVersion: "1.0.0"}, pkgJson: raw.ToNpmPackage(),
			}
			ctx.analyzeSplitting()
			meta, _, err := ctx.buildModule(false)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(meta.Imports, "/example@1.0.0/es2022/state.mjs") {
				t.Fatalf("state must be shared with the public subpath: imports = %v", meta.Imports)
			}
		})
	}
}

func TestRouterDevelopmentRuntimeSubPath(t *testing.T) {
	useNegativeCache(t)
	previousConfig, previousNpmRC, transport := config, defaultNpmRC, http.DefaultTransport
	testConfig := *config
	testConfig.WorkDir = t.TempDir()
	config, defaultNpmRC = &testConfig, nil
	t.Cleanup(func() {
		config, defaultNpmRC, http.DefaultTransport = previousConfig, previousNpmRC, transport
	})
	fs, err := storage.NewFSStorage(filepath.Join(config.WorkDir, "storage"))
	if err != nil {
		t.Fatal(err)
	}
	logger := new(log.Logger)
	logger.SetOutput(io.Discard)
	handler := esmRouter(fs, logger)
	http.DefaultTransport = ghTestTransport(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected request: %s", r.URL)
		return &http.Response{StatusCode: 500, Body: http.NoBody}, nil
	})
	for _, pkg := range []string{"react", "vue"} {
		dir := filepath.Join(config.WorkDir, "npm", pkg+"@99.0.0", "node_modules", pkg)
		for name, content := range map[string]string{
			"package.json":    `{"name":"` + pkg + `","version":"99.0.0","type":"module","types":"./index.d.ts","exports":{".":"./index.js","./jsx-dev-runtime":{"development":"./development.mjs","default":"./production.mjs"}}}`,
			"index.d.ts":      `export declare function jsxDEV(): string;`,
			"index.js":        `export const state = {};`,
			"index.mjs":       `export const alternate = {};`,
			"development.mjs": `export function jsxDEV() { return "development"; }`,
			"production.mjs":  `export const jsxDEV = undefined;`,
		} {
			filename := filepath.Join(dir, name)
			if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
				t.Fatal(err)
			}
		}
		t.Cleanup(func() { deleteCacheItemsWithPrefix("npm:" + pkg + "@99.0.0") })
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest("GET", "/"+pkg+"@99.0.0/index.js?target=es2022&meta", nil))
		if res.Code != 200 || !strings.Contains(res.Body.String(), "/es2022/"+pkg+".mjs") {
			t.Errorf("%s/index.js must use the root module: HTTP %d: %s", pkg, res.Code, res.Body.String())
		}
		for _, subPath := range []string{"jsx-dev-runtime", "jsx-dev-runtime.js", "jsx-dev-runtime.mjs"} {
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, httptest.NewRequest("GET", "/"+pkg+"@99.0.0/"+subPath+"?target=es2022&meta", nil))
			if res.Code != 200 || !strings.Contains(res.Body.String(), "/jsx-dev-runtime.development.mjs") {
				t.Errorf("%s/%s: HTTP %d: %s", pkg, subPath, res.Code, res.Body.String())
			}
		}
	}
}

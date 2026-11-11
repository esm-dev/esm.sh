package server

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/esm-dev/esm.sh/internal/storage"
	esbuild "github.com/ije/esbuild-internal/api"
	"github.com/ije/gox/log"
)

func TestRouterExportExtensionCollision(t *testing.T) {
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
	const pkg = "export-extension-test"
	dir := filepath.Join(config.WorkDir, "npm", pkg+"@1.0.0", "node_modules", pkg)
	for name, content := range map[string]string{
		"package.json":   `{"name":"` + pkg + `","version":"1.0.0","type":"module","types":"./index.d.ts","exports":{".":"./index.mjs","./client":"./lib/client.mjs","./client.mjs":"./client.mjs","./util.js":"./util.js","./dual.js":"./dual.js","./dual.mjs":"./dual.mjs","./wild/*":"./lib/*.mjs","./wild/*.mjs":"./*.mjs","./app.bundle":"./plain.mjs","./app.bundle.js":"./extended.mjs"}}`,
		"index.d.ts":     `export declare class Langsmith {}`,
		"index.mjs":      `export { Langsmith } from "./client.mjs"; export { JS } from "./dual.js"; export { MJS } from "./dual.mjs";`,
		"client.mjs":     `export class Langsmith {}`,
		"lib/client.mjs": `export class Client {}`,
		"util.js":        `export const Util = "util";`,
		"dual.js":        `export const JS = "js";`,
		"dual.mjs":       `export const MJS = "mjs";`,
		"widget.mjs":     `export const InternalWidget = "internal";`,
		"lib/widget.mjs": `export const PublicWidget = "public";`,
		"plain.mjs":      `export const Plain = "plain";`,
		"extended.mjs":   `export const Extended = "extended";`,
	} {
		filename := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { deleteCacheItemsWithPrefix("npm:" + pkg + "@") })
	tests := []struct{ path, export, canonical string }{
		{"/util.js?target=es2022", "Util", "/es2022/util.mjs"},
		{"/es2022/util.mjs", "Util", ""},
		{"/dual.js?target=es2022", "JS", ""},
		{"/dual.mjs?target=es2022", "MJS", ""},
		{"/wild/widget?target=es2022", "PublicWidget", ""},
		{"/wild/widget.mjs?target=es2022", "InternalWidget", ""},
		{"/client.mjs?target=es2022&dev&bundle=false", "Langsmith", "/es2022/client.mjs.development.nobundle.mjs"},
		{"/app.bundle?target=es2022", "Plain", "/es2022/app.bundle.js.mjs"},
		{"/app.bundle.js?target=es2022", "Extended", "/es2022/app.bundle.js.js.mjs"},
		{"/es2022/app.bundle.js.js.mjs", "Extended", ""},
	}
	for _, target := range []string{"deno", "denonext", "es2022"} {
		for _, test := range []struct{ path, export, canonical string }{
			{"?target=" + target, "Langsmith, JS, MJS", ""},
			{"/client?target=" + target, "Client", "/" + target + "/client.mjs"},
			{"/client.mjs?target=" + target, "Langsmith", "/" + target + "/client.mjs.mjs"},
			{"/" + target + "/client.mjs", "Client", ""},
			{"/" + target + "/client.mjs.mjs", "Langsmith", ""},
		} {
			tests = append(tests, test)
		}
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			entry := "http://localhost/" + pkg + "@1.0.0" + test.path
			result := esbuild.Build(esbuild.BuildOptions{
				Stdin:  &esbuild.StdinOptions{Contents: fmt.Sprintf("export { %s } from %q;", test.export, entry)},
				Bundle: true, Format: esbuild.FormatESModule,
				Plugins: []esbuild.Plugin{{Name: "cdn", Setup: func(build esbuild.PluginBuild) {
					build.OnResolve(esbuild.OnResolveOptions{Filter: ".*"}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
						base, err := url.Parse(args.Importer)
						if err != nil {
							return esbuild.OnResolveResult{}, err
						}
						resolved, err := base.Parse(args.Path)
						if err != nil {
							return esbuild.OnResolveResult{}, err
						}
						return esbuild.OnResolveResult{Path: resolved.String(), Namespace: "cdn"}, nil
					})
					build.OnLoad(esbuild.OnLoadOptions{Filter: ".*", Namespace: "cdn"}, func(args esbuild.OnLoadArgs) (esbuild.OnLoadResult, error) {
						res := httptest.NewRecorder()
						handler.ServeHTTP(res, httptest.NewRequest("GET", args.Path, nil))
						if res.Code != 200 {
							return esbuild.OnLoadResult{}, fmt.Errorf("%s: HTTP %d: %s", args.Path, res.Code, res.Body.String())
						}
						if args.Path == entry && test.canonical != "" && res.Header().Get("X-ESM-Path") != "/"+pkg+"@1.0.0"+test.canonical {
							return esbuild.OnLoadResult{}, fmt.Errorf("unexpected canonical path: %s", res.Header().Get("X-ESM-Path"))
						}
						code := res.Body.String()
						return esbuild.OnLoadResult{Contents: &code, Loader: esbuild.LoaderJS}, nil
					})
				}}},
			})
			if len(result.Errors) > 0 {
				t.Fatalf("module graph failed: %v", result.Errors)
			}
		})
	}
}

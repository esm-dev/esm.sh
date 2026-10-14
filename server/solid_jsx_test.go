package server

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/esm-dev/esm.sh/internal/importmap"
	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/esm-dev/esm.sh/internal/storage"
	"github.com/ije/gox/log"
)

func TestBuildModuleSolidJSX(t *testing.T) {
	oldConfig := config
	config = &Config{}
	t.Cleanup(func() { config = oldConfig })
	for _, tt := range []struct {
		name string
		file string
		code string
		fail bool
	}{
		{"jsx", "index.jsx", `export default () => <div />;`, true},
		{"multiple children", "index.jsx", `export default <div>{"one"}{"two"}</div>;`, true},
		{"tsx", "index.tsx", `export default (props: { name: string }) => <div>{props.name}</div>;`, true},
		{"development", "index.jsx", `export default () => <div />;`, true},
		{"nested", "view.jsx", `export default () => <div />;`, true},
		{"compiled", "index.js", `import { template } from "solid-js/web"; export default template("<div></div>");`, false},
		{"no jsx", "index.jsx", `export { createSignal } from "solid-js";`, false},
		{"runtime import", "index.js", `export { createSignal as jsx } from "solid-js/jsx-runtime";`, false},
		{"dev runtime import", "index.js", `export { createRoot } from "solid-js/jsx-dev-runtime";`, false},
		{"namespace import", "index.js", `import * as runtime from "solid-js/jsx-runtime"; export const hasJSX = typeof runtime.jsx === "function";`, false},
		{"mixed runtime import", "index.jsx", `export { createSignal } from "solid-js/jsx-runtime"; export default <div />;`, true},
		{"react with runtime import", "index.jsx", `/** @jsxImportSource react */ export { createSignal } from "solid-js/jsx-runtime"; export default <div />;`, false},
		{"alias", "index.jsx", `export default <div />;`, false},
		{"minified", "index.jsx", `export default () => <div><span /><span /></div>;`, true},
		{"hyperscript", "index.jsx", `/** @jsxImportSource solid-js/h */ export default () => <div />;`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config.Minify = tt.name == "minified"
			wd := t.TempDir()
			pkgDir := filepath.Join(wd, "node_modules", "example")
			if err := os.MkdirAll(pkgDir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(pkgDir, tt.file), []byte(tt.code), 0644); err != nil {
				t.Fatal(err)
			}
			if tt.name == "development" {
				if err := os.WriteFile(filepath.Join(pkgDir, "tsconfig.json"), []byte(`{"compilerOptions":{"jsx":"react-jsxdev","jsxImportSource":"solid-js"}}`), 0644); err != nil {
					t.Fatal(err)
				}
			}
			main := "./" + tt.file
			if tt.name == "nested" {
				main = "./index.js"
				if err := os.WriteFile(filepath.Join(pkgDir, main), []byte(`export { default } from "./view.jsx";`), 0644); err != nil {
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
				wd: wd, target: "es2022", storage: fs, logger: logger, npmrc: &NpmRC{},
				esmPath: EsmPath{PkgName: "example", PkgVersion: "1.0.0"},
				pkgJson: &npm.PackageJSON{Name: "example", Version: "1.0.0", Type: "module", Main: main, Types: "./index.d.ts", PeerDependencies: map[string]string{"solid-js": "1.6.16"}},
			}
			if tt.name == "alias" {
				ctx.args.Alias = map[string]string{"solid-js": "solid-js/h"}
				ctx.externalAll = true
			}
			if tt.name == "react with runtime import" {
				ctx.pkgJson.PeerDependencies["react"] = "18.2.0"
			}
			_, _, err = ctx.buildModule(false)
			if tt.fail {
				if err == nil || !strings.Contains(err.Error(), "Solid JSX is not supported") {
					t.Fatalf("expected a Solid JSX error, got %v", err)
				}
				if _, err := fs.Stat(ctx.getSavePath()); !errors.Is(err, storage.ErrNotFound) {
					t.Fatalf("expected no cached module, got %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTransformSolidJSX(t *testing.T) {
	for _, tt := range []struct {
		name   string
		code   string
		source string
		fail   bool
	}{
		{"explicit", `export default <div />;`, "solid-js", true},
		{"multiple children", `export default <div>{"one"}{"two"}</div>;`, "solid-js", true},
		{"import map", `export default <div />;`, "", true},
		{"pragma", `/** @jsxImportSource solid-js */ export default <div />;`, "react", true},
		{"compiled", `import { template } from "solid-js/web"; export default template("<div></div>");`, "solid-js", false},
		{"runtime import", `export { createSignal as jsx } from "solid-js/jsx-runtime";`, "solid-js", false},
		{"dev runtime import", `export { createRoot } from "solid-js/jsx-dev-runtime";`, "solid-js", false},
		{"namespace import", `import * as runtime from "solid-js/jsx-runtime"; export const hasJSX = typeof runtime.jsx === "function";`, "solid-js", false},
		{"mixed runtime import", `export { createSignal } from "solid-js/jsx-runtime"; export default <div />;`, "solid-js", true},
		{"react with runtime import", `export { createSignal } from "solid-js/jsx-runtime"; export default <div />;`, "react", false},
		{"mapped hyperscript", `export default <div />;`, "solid-js", false},
		{"mapped custom runtime", `export default <div />;`, "solid-js", false},
		{"self-hosted", `export default <div />;`, "solid-js", true},
		{"minified", `export default <div><span /><span /></div>;`, "solid-js", true},
		{"hyperscript", `export default <div />;`, "solid-js/h", false},
		{"react", `export default <div />;`, "react", false},
		{"preact", `export default <div />;`, "preact", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			im := importmap.Blank()
			im.Imports.Set("solid-js/", "https://esm.sh/solid-js@1.6.16/")
			switch tt.name {
			case "mapped hyperscript":
				im.Imports.Set("solid-js/jsx-runtime", "https://esm.sh/solid-js@1.6.16/h/jsx-runtime")
			case "mapped custom runtime":
				im.Imports.Set("solid-js/jsx-runtime", "https://example.com/runtime.js")
			case "self-hosted":
				im.Imports.Set("solid-js/", "http://localhost:8080/solid-js@1.6.16/")
			}
			out, err := transform(&ResolvedTransformOptions{
				TransformOptions: TransformOptions{Lang: "tsx", Code: tt.code, JSXImportSource: tt.source, Minify: tt.name == "minified"},
				importMap:        im,
			})
			if tt.fail {
				if err == nil || !strings.Contains(err.Error(), "Solid JSX is not supported") {
					t.Fatalf("expected a Solid JSX error, got %v; output: %+v", err, out)
				}
				if out != nil {
					t.Fatal("expected no transformed output")
				}
			} else if err != nil {
				t.Fatal(err)
			} else if strings.HasPrefix(tt.name, "mapped ") {
				mapped, _ := im.Imports.Get("solid-js/jsx-runtime")
				if !strings.Contains(out.Code, mapped) {
					t.Fatalf("missing mapped runtime %q: %s", mapped, out.Code)
				}
			}
		})
	}
}
